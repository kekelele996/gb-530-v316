import { Injectable, inject, signal } from '@angular/core';
import { tap } from 'rxjs';
import { AdjustmentsApi } from '../api/adjustments.api';
import { LimitAdjustment, LimitAdjustmentInput } from '../types/permit';

@Injectable({ providedIn: 'root' })
export class AdjustmentsStore {
  private readonly api = inject(AdjustmentsApi);
  readonly adjustments = signal<LimitAdjustment[]>([]);
  readonly workerId = signal(0);
  readonly loading = signal(false);

  load(workerId: number): void {
    if (!workerId) return;
    this.workerId.set(workerId);
    this.loading.set(true);
    this.api.list(workerId).subscribe({
      next: response => { this.adjustments.set(response.data); this.loading.set(false); },
      error: () => this.loading.set(false),
    });
  }

  create(input: LimitAdjustmentInput) {
    const workerId = this.workerId();
    return this.api.create(workerId, input).pipe(tap(() => this.load(workerId)));
  }

  review(id: number, decision: 'approve' | 'reject', note: string) {
    return this.api.review(id, decision, note).pipe(tap(() => this.load(this.workerId())));
  }
}
