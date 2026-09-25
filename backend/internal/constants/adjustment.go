package constants

const (
	AdjustmentStatusPending  = "pending"
	AdjustmentStatusApproved = "approved"
	AdjustmentStatusRejected = "rejected"
)

var AdjustmentStatuses = []string{
	AdjustmentStatusPending,
	AdjustmentStatusApproved,
	AdjustmentStatusRejected,
}

func IsAdjustmentStatus(value string) bool {
	for _, status := range AdjustmentStatuses {
		if value == status {
			return true
		}
	}
	return false
}
