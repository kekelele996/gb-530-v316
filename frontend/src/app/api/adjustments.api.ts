import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { ApiEnvelope } from '../types/api';
import { LimitAdjustment, LimitAdjustmentInput } from '../types/permit';

@Injectable({ providedIn: 'root' })
export class AdjustmentsApi {
  private readonly http = inject(HttpClient);

  list(workerId: number) {
    return this.http.get<ApiEnvelope<LimitAdjustment[]>>(`/api/v1/workers/${workerId}/adjustments`);
  }
  create(workerId: number, input: LimitAdjustmentInput) {
    return this.http.post<ApiEnvelope<LimitAdjustment>>(`/api/v1/workers/${workerId}/adjustments`, input);
  }
  review(id: number, decision: 'approve' | 'reject', note: string) {
    return this.http.post<ApiEnvelope<LimitAdjustment>>(`/api/v1/adjustments/${id}/review`, { decision, note });
  }
}
