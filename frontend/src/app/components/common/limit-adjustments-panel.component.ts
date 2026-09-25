import { ChangeDetectionStrategy, Component, OnInit, effect, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { MatSelectModule } from '@angular/material/select';
import { finalize } from 'rxjs';
import { LimitAdjustmentsStore } from '../../stores/limit-adjustments.store';
import { WorkersStore } from '../../stores/workers.store';
import { useAuth } from '../../hooks/use-auth';
import { apiErrorMessage } from '../../utils/api-error';
import { LimitAdjustmentStatus } from '../../types/limit-adjustment';
import { WorkerProfile } from '../../types/permit';

@Component({
  selector: 'app-limit-adjustments-panel',
  standalone: true,
  imports: [
    CommonModule, ReactiveFormsModule, MatButtonModule, MatFormFieldModule, MatInputModule, MatSelectModule,
  ],
  template: `
    <section class="adjustments surface">
      <header class="adj-head">
        <div><span class="eyebrow">Temporary administrative limits</span><h2>Maintenance-window adjustments</h2>
        <p>Planners propose an RPO-approved override with explicit dates and reason. Only an approved window covering an assessment period end is used; the baseline limit otherwise applies.</p></div>
      </header>
      <p class="error-banner" *ngIf="error()">{{ error() }}</p>

      <form *ngIf="auth.canPlan()" class="adj-form" [formGroup]="form" (ngSubmit)="submit()">
        <mat-form-field appearance="outline"><mat-label>Worker</mat-label>
          <mat-select formControlName="worker_id">
            <mat-option *ngFor="let worker of workers.workers()" [value]="worker.id">
              {{ worker.worker_code }} · {{ worker.display_name }} · legal {{ worker.annual_limit_msv | number:'1.1-1' }} mSv
            </mat-option>
          </mat-select>
        </mat-form-field>
        <mat-form-field appearance="outline"><mat-label>Effective date</mat-label><input matInput type="date" formControlName="effective_date"></mat-form-field>
        <mat-form-field appearance="outline"><mat-label>Expiry date (exclusive)</mat-label><input matInput type="date" formControlName="expiry_date"></mat-form-field>
        <mat-form-field appearance="outline"><mat-label>Adjusted admin mSv</mat-label><input matInput type="number" step="0.1" formControlName="adjusted_limit_msv"></mat-form-field>
        <mat-form-field class="span-2" appearance="outline"><mat-label>Reason</mat-label><textarea matInput rows="2" formControlName="reason"></textarea></mat-form-field>
        <div class="form-actions span-2">
          <button mat-flat-button color="primary" type="submit" [disabled]="form.invalid || saving()">{{ saving() ? 'Submitting' : 'Submit for RPO approval' }}</button>
          <small *ngIf="selectedWorker as worker">Cannot exceed the legal planning limit of {{ worker.annual_limit_msv | number:'1.1-1' }} mSv.</small>
        </div>
      </form>

      <div class="adj-filter">
        <button type="button" [class.active]="statusFilter() === ''" (click)="setStatus('')">All</button>
        <button type="button" [class.active]="statusFilter() === 'pending'" (click)="setStatus('pending')">Pending</button>
        <button type="button" [class.active]="statusFilter() === 'approved'" (click)="setStatus('approved')">Approved</button>
        <button type="button" [class.active]="statusFilter() === 'rejected'" (click)="setStatus('rejected')">Rejected</button>
      </div>

      <table class="data-table">
        <thead><tr><th>Worker</th><th>Window [effective, expiry)</th><th>Adjusted / baseline</th><th>Status</th><th>Reason / RPO note</th><th *ngIf="auth.canReview()">RPO action</th></tr></thead>
        <tbody>
          <tr *ngFor="let item of store.adjustments()">
            <td><strong>{{ item.worker_name }}</strong><br><span class="code muted">{{ item.worker_code }}</span></td>
            <td class="number">{{ item.effective_date | date:'mediumDate':'UTC' }} → {{ item.expiry_date | date:'mediumDate':'UTC' }}</td>
            <td class="number"><strong>{{ item.adjusted_limit_msv | number:'1.2-2' }} mSv</strong></td>
            <td><span class="adj-status" [class.pending]="item.status === 'pending'" [class.approved]="item.status === 'approved'" [class.rejected]="item.status === 'rejected'">{{ item.status }}</span></td>
            <td class="reason-cell">{{ item.reason }}<span class="muted rejection" *ngIf="item.status === 'rejected'"><strong>RPO:</strong> {{ item.rejection_reason }}</span></td>
            <td *ngIf="auth.canReview()">
              <ng-container *ngIf="item.status === 'pending' && reviewingId() !== item.id">
                <button mat-button color="primary" type="button" (click)="startReview(item.id)">Review</button>
              </ng-container>
              <form *ngIf="item.status === 'pending' && reviewingId() === item.id" class="review-inline" [formGroup]="reviewForm" (ngSubmit)="decide(item.id, 'approve')">
                <textarea rows="2" placeholder="Reason required when rejecting" formControlName="rejection_reason"></textarea>
                <div class="review-buttons">
                  <button mat-button color="warn" type="button" [disabled]="saving()" (click)="decide(item.id, 'reject')">Reject</button>
                  <button mat-flat-button color="primary" type="submit" [disabled]="saving()">Approve</button>
                </div>
              </form>
              <span class="muted" *ngIf="item.status !== 'pending'">{{ item.reviewed_at ? ('Reviewed ' + (item.reviewed_at | date:'mediumDate':'UTC')) : '' }}</span>
            </td>
          </tr>
        </tbody>
      </table>
      <div class="empty" *ngIf="!store.loading() && !store.adjustments().length">No temporary limit adjustments match this filter.</div>
    </section>
  `,
  styles: [`
    .adj-head { padding: 16px 18px; border-bottom: 1px solid var(--line); }
    .adj-head h2 { margin: 4px 0; font-size: 16px; }
    .adj-head p { margin: 0; color: var(--muted); font-size: 12px; max-width: 820px; }
    .adj-form { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 4px 14px; padding: 14px 18px; background: #f8f8f3; border-bottom: 1px solid var(--line); }
    .span-2 { grid-column: 1 / -1; }
    .form-actions { display: flex; align-items: center; gap: 14px; }
    .form-actions small { color: var(--muted); }
    .adj-filter { display: flex; gap: 6px; padding: 10px 18px; }
    .adj-filter button { border: 1px solid var(--line); background: #fff; padding: 4px 12px; font-size: 11px; text-transform: uppercase; cursor: pointer; border-radius: 3px; }
    .adj-filter button.active { background: #286858; color: #fff; border-color: #286858; }
    .adj-status { display: inline-block; padding: 3px 8px; border-radius: 3px; font-size: 11px; font-weight: 700; text-transform: capitalize; background: #e3eee9; color: #185847; }
    .adj-status.pending { background: #fff0ce; color: #7b5109; }
    .adj-status.approved { background: #e3eee9; color: #185847; }
    .adj-status.rejected { background: #f6e0de; color: #8a2f27; }
    .reason-cell { font-size: 12px; max-width: 320px; }
    .rejection { display: block; margin-top: 4px; }
    .review-inline { display: grid; gap: 6px; min-width: 220px; }
    .review-inline textarea { width: 100%; border: 1px solid var(--line); padding: 6px; font: inherit; font-size: 12px; }
    .review-buttons { display: flex; justify-content: flex-end; gap: 6px; }
    .number { font-variant-numeric: tabular-nums; white-space: nowrap; }
  `],
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class LimitAdjustmentsPanelComponent implements OnInit {
  readonly store = inject(LimitAdjustmentsStore);
  readonly workers = inject(WorkersStore);
  readonly auth = useAuth();
  private readonly builder = new FormBuilder().nonNullable;
  readonly saving = signal(false);
  readonly error = signal('');
  readonly statusFilter = signal<LimitAdjustmentStatus | ''>('');
  readonly reviewingId = signal<number | null>(null);
  readonly form = this.builder.group({
    worker_id: [0, [Validators.required, Validators.min(1)]],
    effective_date: ['', Validators.required],
    expiry_date: ['', Validators.required],
    adjusted_limit_msv: [12, [Validators.required, Validators.min(0.001)]],
    reason: ['', [Validators.required, Validators.minLength(3), Validators.maxLength(1000)]],
  });
  readonly reviewForm = this.builder.group({
    rejection_reason: ['', Validators.maxLength(1000)],
  });

  ngOnInit(): void {
    this.workers.load();
    this.store.load();
  }

  private readonly defaultWorker = effect(() => {
    const workers = this.workers.workers();
    if (workers.length && !this.form.controls.worker_id.value) {
      this.form.controls.worker_id.setValue(workers[0].id);
    }
  });

  get selectedWorker(): WorkerProfile | undefined {
    return this.workers.workers().find(worker => worker.id === this.form.controls.worker_id.value);
  }

  setStatus(status: LimitAdjustmentStatus | ''): void {
    this.statusFilter.set(status);
    this.store.load(undefined, status || undefined);
  }

  submit(): void {
    if (this.form.invalid) return;
    const value = this.form.getRawValue();
    this.saving.set(true); this.error.set('');
    this.store.create({
      worker_id: value.worker_id,
      effective_date: new Date(`${value.effective_date}T00:00:00Z`).toISOString(),
      expiry_date: new Date(`${value.expiry_date}T00:00:00Z`).toISOString(),
      adjusted_limit_msv: value.adjusted_limit_msv,
      reason: value.reason,
    }).pipe(finalize(() => this.saving.set(false))).subscribe({
      next: () => this.form.controls.reason.reset(''),
      error: error => this.error.set(apiErrorMessage(error)),
    });
  }

  startReview(id: number): void {
    this.reviewingId.set(id);
    this.reviewForm.reset({ rejection_reason: '' });
  }

  decide(id: number, decision: 'approve' | 'reject'): void {
    const reason = this.reviewForm.controls.rejection_reason.value.trim();
    if (decision === 'reject' && reason.length < 3) {
      this.error.set('A rejection reason is required.');
      return;
    }
    this.saving.set(true); this.error.set('');
    this.store.review(id, decision, reason).pipe(finalize(() => this.saving.set(false))).subscribe({
      next: () => { this.reviewingId.set(null); this.reviewForm.reset(); },
      error: error => this.error.set(apiErrorMessage(error)),
    });
  }
}
