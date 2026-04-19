#!/bin/bash

set -e

# Function to get network interfaces from RDMA links and filter out VFs
get_rdma_interfaces() {
    local interfaces=()
    
    # Check if rdma command exists
    if ! command -v rdma &> /dev/null; then
        echo "Warning: rdma command not found, skipping RDMA interface detection"
        return 0
    fi
    
    # Get RDMA link Information
    local rdma_int_list
    rdma_int_list=$(rdma link | grep "netdev" | grep -v "DOWN" | grep -oE "netdev .*" | awk '{print $2}')
    
    if [ -z "$rdma_int_list" ]; then
        echo "Warning: Failed to get RDMA link Information"
        return 0
    fi
    
    for rdma_link in $rdma_int_list; do
        if [ -d /sys/class/net/$rdma_link/device/physfn ]; then
            continue
        fi
        interfaces+=($rdma_link)
    done
    
    # Remove duplicates and sort
    if [ ${#interfaces[@]} -gt 0 ]; then
        printf '%s\n' "${interfaces[@]}" | sort -u
    fi
}

# Function to update lldpd configuration
update_lldpd_config() {
    local config_file="/etc/lldpd.d/lldpd.conf"
    unifabric_lldpd_file="/usr/bin/unifabric/lldpd.conf"
    
    # Create config directory if it doesn't exist
    mkdir -p "/etc/lldpd.d"
    # Create config file if it doesn't exist
    touch "$unifabric_lldpd_file"
    [ ! -f "$config_file" ] && touch "$config_file"

    # Determine interface pattern
    local interface_pattern

    # Check if LLDPD_INTERFACE_PATTERN environment variable is set
    if [ -n "$LLDPD_INTERFACE_PATTERN" ]; then
        interface_pattern="$LLDPD_INTERFACE_PATTERN"
        echo "Info, using interface pattern from environment variable: $interface_pattern"
    else
        # Get RDMA interfaces
        local interfaces
        mapfile -t interfaces < <(get_rdma_interfaces)

        # Update interface pattern if we have interfaces
        if [ ${#interfaces[@]} -gt 0 ]; then
            echo "Info, found RDMA interfaces: ${interfaces[*]}"

            # Create interface pattern (comma-separated)
            interface_pattern=$(IFS=','; echo "${interfaces[*]}")
        else
            echo "error, no RDMA interfaces found"
            exit 1
        fi
    fi
    
    # Remove existing interface pattern line
    grep -v "^configure system interface pattern" "$config_file" > "$unifabric_lldpd_file" || true
    
    # Add new interface pattern
    echo "configure system interface pattern $interface_pattern" >> "$unifabric_lldpd_file"
    echo "Info, updated lldpd interface pattern: $interface_pattern"

    # Check and update management IP pattern if NODE_IPADDRESS is set
    if [ -n "$NODE_IPADDRESS" ] ; then 
        grep -v "^configure system ip management pattern" "$unifabric_lldpd_file" > "$unifabric_lldpd_file.tmp" || true
        mv "$unifabric_lldpd_file.tmp" "$unifabric_lldpd_file"
        echo "configure system ip management pattern $NODE_IPADDRESS" >> "$unifabric_lldpd_file"
        echo "Info, added management IP pattern: $NODE_IPADDRESS"
    fi

    # If NODE_NAME is unset or empty, try read /etc/hostname (for storage nodes joining via docker compose)
    if [ -z "$NODE_NAME" ]; then
        if [ -s /etc/hostname ]; then
            NODE_NAME=$(cat /etc/hostname | tr -d '[:space:]')
        fi
    fi
    if [ -n "$NODE_NAME" ] ; then 
        grep -v "^configure system name" "$unifabric_lldpd_file" > "$unifabric_lldpd_file.tmp" || true
        mv "$unifabric_lldpd_file.tmp" "$unifabric_lldpd_file"
        echo "configure system hostname $NODE_NAME" >> "$unifabric_lldpd_file"
        echo "Info, added system hostname: $NODE_NAME"
    fi

    if [ -n "$LLDPD_TX_INTERVAL" ] ; then 
        grep -v "^configure lldp tx-interval" "$unifabric_lldpd_file" > "$unifabric_lldpd_file.tmp" || true
        mv "$unifabric_lldpd_file.tmp" "$unifabric_lldpd_file"
        echo "configure lldp tx-interval $LLDPD_TX_INTERVAL" >> "$unifabric_lldpd_file"
        echo "Info, added tx-interval: $LLDPD_TX_INTERVAL"
    fi

    # Display final configuration
    echo "Final lldpd configuration:"
    echo "----------"
    cat "$unifabric_lldpd_file"
    echo "----------"
}

# Main execution
update_lldpd_config

# Start lldpd service if requested
echo "lldpd -d -O $unifabric_lldpd_file"
lldpd -d -O $unifabric_lldpd_file
