export type LimitAdjustmentStatus = 'pending' | 'approved' | 'rejected';

export interface TemporaryLimitAdjustment {
  id: number;
  worker_id: number;
  worker_code: string;
  worker_name: string;
  status: LimitAdjustmentStatus;
  effective_date: string;
  expiry_date: string;
  adjusted_limit_msv: number;
  reason: string;
  created_by: number;
  created_at: string;
  reviewed_by?: number;
  reviewed_at?: string;
  rejection_reason: string;
}

export interface LimitAdjustmentInput {
  worker_id: number;
  effective_date: string;
  expiry_date: string;
  adjusted_limit_msv: number;
  reason: string;
}
