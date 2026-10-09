// SPDX-License-Identifier: GPL-2.0
/*
 * rdma_flow: pod-to-pod RDMA flow accounting MVP.
 *
 * Control plane: the ibv_modify_qp entry uprobe records each user QP's
 * destination GID when a request supplies an address and the RTR state.
 * The same probe also discovers the provider's post_send callback.
 * Bootstrap: uprobes on public libibverbs functions read provider callback
 * addresses from live verbs objects. User space converts those virtual
 * addresses to file offsets and attaches the data-plane probes without
 * requiring provider symbols.
 * Data plane: uprobes on the discovered provider callbacks count payload
 * bytes per (tgid, qpn) on both the legacy and extended verbs APIs.
 * User space joins the two maps into pod->pod byte totals.
 * qp_infos and qp_stats are pinned by name so an agent restart reuses the
 * QP destinations and byte totals learned by the previous instance.
 */
#include <linux/bpf.h>
#include <linux/types.h>
#include <asm/ptrace.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

char LICENSE[] SEC("license") = "GPL";

#define MAX_WR_CHAIN 16
#define MAX_SGE 16

/* libibverbs public QP attribute mask and state values. */
#define IBV_QP_STATE_MASK 1
#define IBV_QP_AV_MASK (1 << 7)
#define IBV_QP_DEST_QPN_MASK (1 << 20)
#define IBV_QPS_RTR_VALUE 2

/* libibverbs public ABI offsets on 64-bit. */
#define IBV_QP_CONTEXT_OFFSET 0
#define IBV_QP_QP_NUM_OFFSET 52
#define IBV_CONTEXT_DEVICE_OFFSET 0
#define IBV_DEVICE_NAME_OFFSET 24
#define IBV_SYSFS_NAME_MAX 64
#define IBV_QP_ATTR_STATE_OFFSET 0
#define IBV_QP_ATTR_DEST_QPN_OFFSET 28
/* struct ibv_ah_attr starts at 48: grh.dgid 56, dlid 80, is_global 85,
 * port_num 86. */
#define IBV_QP_ATTR_DGID_OFFSET 56
#define IBV_QP_ATTR_DLID_OFFSET 80
#define IBV_QP_ATTR_IS_GLOBAL_OFFSET 85
#define IBV_QP_ATTR_PORT_NUM_OFFSET 86
#define IBV_WR_NEXT_OFFSET 8
#define IBV_WR_SG_LIST_OFFSET 16
#define IBV_WR_NUM_SGE_OFFSET 24
#define IBV_WR_OPCODE_OFFSET 28
#define IBV_SGE_SIZE 16
#define IBV_SGE_LENGTH_OFFSET 8
#define IBV_DATA_BUF_SIZE 16
#define IBV_DATA_BUF_LENGTH_OFFSET 8

/* Frozen libibverbs ABI offsets on 64-bit architectures. */
#define IBV_QP_CONTEXT_OFFSET 0
#define IBV_CONTEXT_POST_SEND_OFFSET 208
#define IBV_QP_EX_SET_INLINE_DATA_OFFSET 288
#define IBV_QP_EX_SET_INLINE_DATA_LIST_OFFSET 296
#define IBV_QP_EX_SET_SGE_OFFSET 304
#define IBV_QP_EX_SET_SGE_LIST_OFFSET 312

enum callback_kind {
	CALLBACK_PROCESS_EXEC = 0,
	CALLBACK_POST_SEND = 1,
	CALLBACK_SET_SGE = 2,
	CALLBACK_SET_SGE_LIST = 3,
	CALLBACK_SET_INLINE_DATA = 4,
	CALLBACK_SET_INLINE_DATA_LIST = 5,
	/* A QP reached RTR and qp_infos was updated. address carries the QPN so
	 * user space can publish the node report without polling the map. */
	CALLBACK_QP_RTR = 6,
};

struct qp_key {
	__u32 tgid;
	__u32 qpn;
};

/* dest_qpn and device let the peer node's report identify the receiving
 * process: QPNs are unique per HCA, so (device, qpn) names one QP there.
 * InfiniBand peers inside one subnet are addressed by dlid without a GRH,
 * so dlid, is_global and the local port are kept to resolve them through
 * the LID table of the peer node's report. */
struct qp_info {
	__u8 dgid[16];
	char comm[16];
	__u32 dest_qpn;
	__u16 dlid;
	__u8 is_global;
	__u8 port_num;
	char device[IBV_SYSFS_NAME_MAX];
};

struct qp_stat {
	__u64 bytes;
	__u64 wrs;
};

struct callback_event {
	__u32 tgid;
	__u32 kind;
	__u64 address;
};

/* One data plane submission. Emitted per post_send call or per extended
 * verbs setter so user space can record the size distribution and stream
 * submissions to a store. The Go side attributes tgid and qpn to Pods. */
struct send_event {
	__u64 ts_ns;
	__u32 tgid;
	__u32 qpn;
	__u64 bytes;
	__u32 wrs;
	__u32 kind;
};

struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, 8192);
	__uint(pinning, LIBBPF_PIN_BY_NAME);
	__type(key, struct qp_key);
	__type(value, struct qp_info);
} qp_infos SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, 8192);
	__uint(pinning, LIBBPF_PIN_BY_NAME);
	__type(key, struct qp_key);
	__type(value, struct qp_stat);
} qp_stats SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 256 * 1024);
} callback_events SEC(".maps");

/* Data plane events are high rate, so they get their own buffer and never
 * starve the control plane events above. Dropped events are counted by the
 * consumer through ringbuf statistics, the map itself never blocks. */
struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 16 * 1024 * 1024);
} send_events SEC(".maps");

struct flow_config {
	__u32 emit_send_events;
};

struct {
	__uint(type, BPF_MAP_TYPE_ARRAY);
	__uint(max_entries, 1);
	__type(key, __u32);
	__type(value, struct flow_config);
} flow_config SEC(".maps");

static __always_inline void emit_callback(__u32 kind, void *address)
{
	struct callback_event *event;

	if (!address)
		return;
	event = bpf_ringbuf_reserve(&callback_events, sizeof(*event), 0);
	if (!event)
		return;
	event->tgid = bpf_get_current_pid_tgid() >> 32;
	event->kind = kind;
	event->address = (__u64)address;
	bpf_ringbuf_submit(event, 0);
}

SEC("tracepoint/sched/sched_process_exec")
int handle_process_exec(void *ctx)
{
	struct callback_event *event;

	event = bpf_ringbuf_reserve(&callback_events, sizeof(*event), 0);
	if (!event)
		return 0;
	event->tgid = bpf_get_current_pid_tgid() >> 32;
	event->kind = CALLBACK_PROCESS_EXEC;
	event->address = 0;
	bpf_ringbuf_submit(event, 0);
	return 0;
}

/* struct ibv_qp_ex embeds struct ibv_qp at offset 0, so the QPN offset holds
 * for both the legacy and the extended entry points. */
static __always_inline int qp_user_key(void *ibqp, struct qp_key *key)
{
	if (bpf_probe_read_user(&key->qpn, sizeof(key->qpn),
				(char *)ibqp + IBV_QP_QP_NUM_OFFSET))
		return -1;
	key->tgid = bpf_get_current_pid_tgid() >> 32;
	return 0;
}

/* Best effort copy of ibv_qp->context->device->name, e.g. "mlx5_0".
 * Failures leave the name empty and the QP is still recorded. */
static __always_inline void read_qp_device(void *ibqp, char *name)
{
	void *ibctx = NULL;
	void *ibdev = NULL;

	if (bpf_probe_read_user(&ibctx, sizeof(ibctx),
				(char *)ibqp + IBV_QP_CONTEXT_OFFSET) || !ibctx)
		return;
	if (bpf_probe_read_user(&ibdev, sizeof(ibdev),
				(char *)ibctx + IBV_CONTEXT_DEVICE_OFFSET) || !ibdev)
		return;
	bpf_probe_read_user_str(name, IBV_SYSFS_NAME_MAX,
				(char *)ibdev + IBV_DEVICE_NAME_OFFSET);
}

static __always_inline void discover_context(void *ibctx)
{
	void *post_send = NULL;

	if (!ibctx)
		return;
	bpf_probe_read_user(&post_send, sizeof(post_send),
			    (char *)ibctx + IBV_CONTEXT_POST_SEND_OFFSET);
	/* Report the provider post_send address to user space so it can attach
	 * the data plane uprobe. */
	emit_callback(CALLBACK_POST_SEND, post_send);
}

static __always_inline void discover_qp(void *ibqp)
{
	void *ibctx = NULL;

	if (!ibqp)
		return;
	bpf_probe_read_user(&ibctx, sizeof(ibctx),
			    (char *)ibqp + IBV_QP_CONTEXT_OFFSET);
	discover_context(ibctx);
}

SEC("uretprobe")
int handle_context_return(struct pt_regs *ctx)
{
	discover_context((void *)PT_REGS_RC(ctx));
	return 0;
}

SEC("uretprobe")
int handle_qp_return(struct pt_regs *ctx)
{
	discover_qp((void *)PT_REGS_RC(ctx));
	return 0;
}

SEC("uprobe")
int BPF_KPROBE(handle_modify_qp, void *ibqp, void *attr, int attr_mask)
{
	struct qp_key key = {};
	struct qp_info info = {};
	__u32 qp_state = 0;

	discover_qp(ibqp);
	if (!(attr_mask & IBV_QP_STATE_MASK))
		return 0;
	if (bpf_probe_read_user(&qp_state, sizeof(qp_state),
				(char *)attr + IBV_QP_ATTR_STATE_OFFSET))
		return 0;
	if (qp_state != IBV_QPS_RTR_VALUE)
		return 0;
	if (!(attr_mask & IBV_QP_AV_MASK))
		return 0;
	if (qp_user_key(ibqp, &key))
		return 0;
	if (bpf_probe_read_user(&info.dgid, sizeof(info.dgid),
				(char *)attr + IBV_QP_ATTR_DGID_OFFSET))
		return 0;
	bpf_probe_read_user(&info.dlid, sizeof(info.dlid),
			    (char *)attr + IBV_QP_ATTR_DLID_OFFSET);
	bpf_probe_read_user(&info.is_global, sizeof(info.is_global),
			    (char *)attr + IBV_QP_ATTR_IS_GLOBAL_OFFSET);
	bpf_probe_read_user(&info.port_num, sizeof(info.port_num),
			    (char *)attr + IBV_QP_ATTR_PORT_NUM_OFFSET);
	if (attr_mask & IBV_QP_DEST_QPN_MASK)
		bpf_probe_read_user(&info.dest_qpn, sizeof(info.dest_qpn),
				    (char *)attr + IBV_QP_ATTR_DEST_QPN_OFFSET);
	read_qp_device(ibqp, info.device);
	bpf_get_current_comm(info.comm, sizeof(info.comm));
	if (!bpf_map_update_elem(&qp_infos, &key, &info, BPF_ANY))
		emit_callback(CALLBACK_QP_RTR, (void *)(__u64)key.qpn);
	return 0;
}

SEC("uretprobe")
int handle_qp_ex_return(struct pt_regs *ctx)
{
	void *ibqp_ex = (void *)PT_REGS_RC(ctx);
	void *callback = NULL;

	if (!ibqp_ex)
		return 0;

	discover_qp(ibqp_ex);
	bpf_probe_read_user(&callback, sizeof(callback),
			    (char *)ibqp_ex + IBV_QP_EX_SET_SGE_OFFSET);
	emit_callback(CALLBACK_SET_SGE, callback);
	bpf_probe_read_user(&callback, sizeof(callback),
			    (char *)ibqp_ex + IBV_QP_EX_SET_SGE_LIST_OFFSET);
	emit_callback(CALLBACK_SET_SGE_LIST, callback);
	bpf_probe_read_user(&callback, sizeof(callback),
			    (char *)ibqp_ex + IBV_QP_EX_SET_INLINE_DATA_OFFSET);
	emit_callback(CALLBACK_SET_INLINE_DATA, callback);
	bpf_probe_read_user(&callback, sizeof(callback),
			    (char *)ibqp_ex + IBV_QP_EX_SET_INLINE_DATA_LIST_OFFSET);
	emit_callback(CALLBACK_SET_INLINE_DATA_LIST, callback);
	return 0;
}

static __always_inline int wr_carries_data(__u32 opcode)
{
	/* WRITE, WRITE_IMM, SEND, SEND_IMM, READ, SEND_WITH_INV */
	return opcode <= 4 || opcode == 9;
}

static __always_inline void emit_send(const struct qp_key *key, __u64 bytes,
				      __u32 wrs, __u32 kind)
{
	__u32 config_key = 0;
	struct flow_config *config;
	struct send_event *event;

	config = bpf_map_lookup_elem(&flow_config, &config_key);
	if (!config || !config->emit_send_events)
		return;
	event = bpf_ringbuf_reserve(&send_events, sizeof(*event), 0);
	if (!event)
		return;
	event->ts_ns = bpf_ktime_get_ns();
	event->tgid = key->tgid;
	event->qpn = key->qpn;
	event->bytes = bytes;
	event->wrs = wrs;
	event->kind = kind;
	bpf_ringbuf_submit(event, 0);
}

static __always_inline void add_stat(const struct qp_key *key, __u64 bytes,
				     __u64 wrs, __u32 kind)
{
	struct qp_stat *stat;
	struct qp_stat zero = {};

	stat = bpf_map_lookup_elem(&qp_stats, key);
	if (!stat) {
		bpf_map_update_elem(&qp_stats, key, &zero, BPF_NOEXIST);
		stat = bpf_map_lookup_elem(&qp_stats, key);
		if (!stat)
			return;
	}
	__sync_fetch_and_add(&stat->bytes, bytes);
	__sync_fetch_and_add(&stat->wrs, wrs);
	emit_send(key, bytes, (__u32)wrs, kind);
}

SEC("uprobe")
int BPF_KPROBE(handle_post_send, void *ibqp, void *wr)
{
	struct qp_key key = {};
	__u64 bytes = 0;
	__u64 wrs = 0;
	void *cur = wr;
	int i;

	if (qp_user_key(ibqp, &key))
		return 0;

	for (i = 0; i < MAX_WR_CHAIN && cur; i++) {
		void *next = NULL;
		void *sg_list = NULL;
		__s32 num_sge = 0;
		__u32 opcode = 0;
		int j;

		bpf_probe_read_user(&next, sizeof(next),
				    (char *)cur + IBV_WR_NEXT_OFFSET);
		bpf_probe_read_user(&sg_list, sizeof(sg_list),
				    (char *)cur + IBV_WR_SG_LIST_OFFSET);
		bpf_probe_read_user(&num_sge, sizeof(num_sge),
				    (char *)cur + IBV_WR_NUM_SGE_OFFSET);
		bpf_probe_read_user(&opcode, sizeof(opcode),
				    (char *)cur + IBV_WR_OPCODE_OFFSET);

		if (wr_carries_data(opcode) && sg_list) {
			for (j = 0; j < MAX_SGE; j++) {
				__u32 len = 0;

				if (j >= num_sge)
					break;
				bpf_probe_read_user(&len, sizeof(len),
						    (char *)sg_list +
						    (__u64)j * IBV_SGE_SIZE +
						    IBV_SGE_LENGTH_OFFSET);
				bytes += len;
			}
		}
		wrs++;
		cur = next;
	}

	if (!wrs)
		return 0;

	add_stat(&key, bytes, wrs, CALLBACK_POST_SEND);
	return 0;
}

/* Extended ibv_wr_* path: the mlx5 data setters run exactly once per WR and
 * carry the payload size in their arguments. Bytes are counted at setter
 * time; WRs dropped later by ibv_wr_abort() are rare and still counted. */

SEC("uprobe")
int BPF_KPROBE(handle_wr_set_sge, void *ibqp_ex, __u32 lkey, __u64 addr,
	       __u32 length)
{
	struct qp_key key = {};

	if (qp_user_key(ibqp_ex, &key))
		return 0;
	add_stat(&key, length, 1, CALLBACK_SET_SGE);
	return 0;
}

SEC("uprobe")
int BPF_KPROBE(handle_wr_set_sge_list, void *ibqp_ex, __u64 num_sge,
	       void *sg_list)
{
	struct qp_key key = {};
	__u64 bytes = 0;
	int j;

	if (!sg_list || qp_user_key(ibqp_ex, &key))
		return 0;
	for (j = 0; j < MAX_SGE; j++) {
		__u32 len = 0;

		if ((__u64)j >= num_sge)
			break;
		bpf_probe_read_user(&len, sizeof(len),
				    (char *)sg_list + (__u64)j * IBV_SGE_SIZE +
				    IBV_SGE_LENGTH_OFFSET);
		bytes += len;
	}
	add_stat(&key, bytes, 1, CALLBACK_SET_SGE_LIST);
	return 0;
}

SEC("uprobe")
int BPF_KPROBE(handle_wr_set_inline_data, void *ibqp_ex, void *addr,
	       __u64 length)
{
	struct qp_key key = {};

	if (qp_user_key(ibqp_ex, &key))
		return 0;
	add_stat(&key, length, 1, CALLBACK_SET_INLINE_DATA);
	return 0;
}

SEC("uprobe")
int BPF_KPROBE(handle_wr_set_inline_data_list, void *ibqp_ex, __u64 num_buf,
	       void *buf_list)
{
	struct qp_key key = {};
	__u64 bytes = 0;
	int j;

	if (!buf_list || qp_user_key(ibqp_ex, &key))
		return 0;
	for (j = 0; j < MAX_SGE; j++) {
		__u64 len = 0;

		if ((__u64)j >= num_buf)
			break;
		bpf_probe_read_user(&len, sizeof(len),
				    (char *)buf_list +
				    (__u64)j * IBV_DATA_BUF_SIZE +
				    IBV_DATA_BUF_LENGTH_OFFSET);
		bytes += len;
	}
	add_stat(&key, bytes, 1, CALLBACK_SET_INLINE_DATA_LIST);
	return 0;
}
