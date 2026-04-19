// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package types

import "context"

type Service interface {
	Start(context.Context) error
}
