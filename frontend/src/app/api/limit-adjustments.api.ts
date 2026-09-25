import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { ApiEnvelope, PageEnvelope } from '../types/api';
import { LimitAdjustmentInput, LimitAdjustmentStatus, TemporaryLimitAdjustment } from '../types/limit-adjustment';

@Injectable({ providedIn: 'root' })
export class LimitAdjustmentsApi {
  private readonly http = inject(HttpClient);
  private readonly root = '/api/v1/limit-adjustments';

  list(workerId?: number, status?: LimitAdjustmentStatus) {
    let params: Record<string, string | number> = { page_size: 100 };
    if (workerId) params = { ...params, worker_id: workerId };
    if (status) params = { ...params, status };
    return this.http.get<PageEnvelope<TemporaryLimitAdjustment>>(this.root, { params });
  }
  create(input: LimitAdjustmentInput) {
    return this.http.post<ApiEnvelope<TemporaryLimitAdjustment>>(this.root, input);
  }
  review(id: number, decision: 'approve' | 'reject', rejectionReason: string) {
    return this.http.post<ApiEnvelope<TemporaryLimitAdjustment>>(`${this.root}/${id}/review`, {
      decision,
      rejection_reason: rejectionReason,
    });
  }
}
