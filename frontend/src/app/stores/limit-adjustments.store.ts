import { Injectable, inject, signal } from '@angular/core';
import { tap } from 'rxjs';
import { LimitAdjustmentsApi } from '../api/limit-adjustments.api';
import { LimitAdjustmentInput, LimitAdjustmentStatus, TemporaryLimitAdjustment } from '../types/limit-adjustment';

@Injectable({ providedIn: 'root' })
export class LimitAdjustmentsStore {
  private readonly api = inject(LimitAdjustmentsApi);
  readonly adjustments = signal<TemporaryLimitAdjustment[]>([]);
  readonly loading = signal(false);

  load(workerId?: number, status?: LimitAdjustmentStatus): void {
    this.loading.set(true);
    this.api.list(workerId, status).subscribe({
      next: response => { this.adjustments.set(response.data); this.loading.set(false); },
      error: () => this.loading.set(false),
    });
  }

  create(input: LimitAdjustmentInput) {
    return this.api.create(input).pipe(tap(response => this.adjustments.update(items => [response.data, ...items])));
  }

  review(id: number, decision: 'approve' | 'reject', rejectionReason: string) {
    return this.api.review(id, decision, rejectionReason).pipe(
      tap(response => this.adjustments.update(items => items.map(item => item.id === id ? response.data : item))),
    );
  }
}
