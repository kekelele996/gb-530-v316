package constants

const (
	LimitAdjustmentStatusPending  = "pending"
	LimitAdjustmentStatusApproved = "approved"
	LimitAdjustmentStatusRejected = "rejected"
)

var LimitAdjustmentStatuses = []string{
	LimitAdjustmentStatusPending,
	LimitAdjustmentStatusApproved,
	LimitAdjustmentStatusRejected,
}

func IsLimitAdjustmentStatus(value string) bool {
	for _, candidate := range LimitAdjustmentStatuses {
		if value == candidate {
			return true
		}
	}
	return false
}
