import { ChangeDetectionStrategy, Component, OnInit, effect, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { MatSelectModule } from '@angular/material/select';
import { finalize } from 'rxjs';
import { WorkersStore } from '../stores/workers.store';
import { AdjustmentsStore } from '../stores/adjustments.store';
import { useAuth } from '../hooks/use-auth';
import { SafetyBoundaryBannerComponent } from '../components/common/safety-boundary-banner.component';
import { apiErrorMessage } from '../utils/api-error';
import { LimitAdjustment, LimitAdjustmentInput, ProfileStatus, WorkerInput } from '../types/permit';

@Component({
  standalone: true,
  imports: [
    CommonModule, ReactiveFormsModule, MatButtonModule, MatFormFieldModule, MatInputModule,
    MatSelectModule, SafetyBoundaryBannerComponent,
  ],
  template: `
    <div class="page">
      <header class="page-head">
        <div><span class="eyebrow">Authorization context</span><h1>Worker dose profiles</h1><p>Minimum planning identity, configured limits and confirmed period totals.</p></div>
        <button *ngIf="auth.canPlan()" mat-flat-button color="primary" (click)="formOpen.set(!formOpen())">{{ formOpen() ? 'Close form' : 'Add profile' }}</button>
      </header>
      <app-safety-boundary-banner />
      <p class="error-banner" *ngIf="error()">{{ error() }}</p>
      <form *ngIf="formOpen()" class="inline-form" [formGroup]="form" (ngSubmit)="create()">
        <mat-form-field class="span-2" appearance="outline"><mat-label>Worker code</mat-label><input matInput formControlName="worker_code"></mat-form-field>
        <mat-form-field class="span-3" appearance="outline"><mat-label>Display name</mat-label><input matInput formControlName="display_name"></mat-form-field>
        <mat-form-field class="span-3" appearance="outline"><mat-label>Authorization level</mat-label><input matInput formControlName="authorization_level"></mat-form-field>
        <mat-form-field class="span-2" appearance="outline"><mat-label>Administrative mSv</mat-label><input matInput type="number" step="0.1" formControlName="administrative_limit_msv"></mat-form-field>
        <mat-form-field class="span-2" appearance="outline"><mat-label>Annual legal mSv</mat-label><input matInput type="number" step="0.1" formControlName="annual_limit_msv"></mat-form-field>
        <mat-form-field class="span-3" appearance="outline"><mat-label>Period start</mat-label><input matInput type="date" formControlName="period_start"></mat-form-field>
        <mat-form-field class="span-3" appearance="outline"><mat-label>Profile status</mat-label><mat-select formControlName="profile_status"><mat-option value="active">Active</mat-option><mat-option value="suspended">Suspended</mat-option><mat-option value="archived">Archived</mat-option></mat-select></mat-form-field>
        <div class="span-6 form-actions"><button mat-button type="button" (click)="formOpen.set(false)">Cancel</button><button mat-flat-button color="primary" type="submit" [disabled]="form.invalid || saving()">{{ saving() ? 'Saving' : 'Create profile' }}</button></div>
      </form>
      <section class="metric-strip">
        <div class="metric"><strong>{{ store.workers().length }}</strong><span>Profiles</span></div>
        <div class="metric"><strong>{{ activeCount }}</strong><span>Active</span></div>
        <div class="metric"><strong>{{ totalDose | number:'1.3-3' }}</strong><span>Confirmed mSv</span></div>
      </section>
      <div class="section-title"><h2>Current authorization set</h2><span>Period totals use verified records only</span></div>
      <div class="surface">
        <table class="data-table">
          <thead><tr><th>Worker</th><th>Authorization</th><th>Status</th><th>Period dose</th><th>Admin margin</th><th>Legal margin</th><th>Period start</th></tr></thead>
          <tbody>
            <tr *ngFor="let worker of store.workers()">
              <td><strong>{{ worker.display_name }}</strong><br><span class="code muted">{{ worker.worker_code }}</span></td>
              <td>{{ worker.authorization_level }}</td>
              <td><span class="status" [class.warn]="worker.profile_status !== 'active'">{{ worker.profile_status }}</span></td>
              <td class="number">{{ worker.period_dose_msv | number:'1.3-3' }} mSv</td>
              <td class="number">{{ worker.remaining_admin_msv | number:'1.3-3' }} mSv</td>
              <td class="number">{{ worker.remaining_legal_msv | number:'1.3-3' }} mSv</td>
              <td>{{ worker.period_start | date:'mediumDate':'UTC' }}</td>
            </tr>
          </tbody>
        </table>
        <div class="empty" *ngIf="!store.loading() && !store.workers().length">No worker profiles match the current planning set.</div>
      </div>
      <div class="section-title">
        <h2>Temporary limit adjustments</h2>
        <span>Outage windows raise the administrative limit only after RPO approval; the legal limit still caps every value</span>
      </div>
      <div class="adjustment-bar surface">
        <mat-form-field appearance="outline"><mat-label>Worker</mat-label><mat-select [value]="adjustments.workerId()" (selectionChange)="selectWorker($event.value)"><mat-option *ngFor="let worker of store.workers()" [value]="worker.id">{{ worker.worker_code }} · {{ worker.display_name }}</mat-option></mat-select></mat-form-field>
        <button *ngIf="auth.canPlan()" mat-flat-button color="primary" type="button" (click)="adjustmentFormOpen.set(!adjustmentFormOpen())">{{ adjustmentFormOpen() ? 'Close request' : 'Request adjustment' }}</button>
      </div>
      <form *ngIf="adjustmentFormOpen()" class="inline-form" [formGroup]="adjustmentForm" (ngSubmit)="requestAdjustment()">
        <mat-form-field class="span-2" appearance="outline"><mat-label>Effective from</mat-label><input matInput type="date" formControlName="effective_from"></mat-form-field>
        <mat-form-field class="span-2" appearance="outline"><mat-label>Effective to</mat-label><input matInput type="date" formControlName="effective_to"></mat-form-field>
        <mat-form-field class="span-2" appearance="outline"><mat-label>Adjusted admin mSv</mat-label><input matInput type="number" min="0" step="0.1" formControlName="adjusted_limit_msv"></mat-form-field>
        <mat-form-field class="span-6" appearance="outline"><mat-label>Reason (outage / maintenance justification)</mat-label><input matInput formControlName="reason"></mat-form-field>
        <div class="span-6 form-actions"><button mat-button type="button" (click)="adjustmentFormOpen.set(false)">Cancel</button><button mat-flat-button color="primary" type="submit" [disabled]="adjustmentForm.invalid || saving()">{{ saving() ? 'Submitting' : 'Submit for RPO review' }}</button></div>
      </form>
      <form *ngIf="reviewTarget() as target" class="review-strip" [formGroup]="reviewForm">
        <div class="review-title"><span class="eyebrow">RPO review · adjustment #{{ target.id }}</span><strong>{{ target.adjusted_limit_msv | number:'1.2-3' }} mSv · {{ target.effective_from | date:'mediumDate':'UTC' }} – {{ target.effective_to | date:'mediumDate':'UTC' }}</strong></div>
        <mat-form-field appearance="outline"><mat-label>Review note (required to reject)</mat-label><input matInput formControlName="note"></mat-form-field>
        <div class="form-actions">
          <button mat-button type="button" (click)="reviewTarget.set(null)">Cancel</button>
          <button mat-button color="warn" type="button" [disabled]="!reviewForm.controls.note.value.trim() || saving()" (click)="review('reject')">Reject</button>
          <button mat-flat-button color="primary" type="button" [disabled]="saving()" (click)="review('approve')">Approve</button>
        </div>
      </form>
      <div class="surface">
        <table class="data-table">
          <thead><tr><th>Validity window</th><th>Adjusted limit</th><th>Reason</th><th>Status</th><th>RPO disposition</th><th *ngIf="auth.canReview()">Actions</th></tr></thead>
          <tbody>
            <tr *ngFor="let item of adjustments.adjustments()" [class.selected]="reviewTarget()?.id === item.id">
              <td>{{ item.effective_from | date:'mediumDate':'UTC' }} – {{ item.effective_to | date:'mediumDate':'UTC' }}</td>
              <td class="number">{{ item.adjusted_limit_msv | number:'1.2-3' }} mSv</td>
              <td>{{ item.reason }}</td>
              <td><span class="adjustment-status" [attr.data-status]="item.status">{{ item.status }}</span></td>
              <td><span *ngIf="item.review_note; else noDisposition">{{ item.review_note }}<br><span class="muted">{{ item.reviewed_at | date:'medium':'UTC' }}</span></span><ng-template #noDisposition><span class="muted">Awaiting RPO review</span></ng-template></td>
              <td *ngIf="auth.canReview()" class="actions"><button *ngIf="item.status === 'pending'" mat-button type="button" (click)="openReview(item)">Review</button></td>
            </tr>
          </tbody>
        </table>
        <div class="empty" *ngIf="!adjustments.loading() && !adjustments.adjustments().length">No temporary adjustments requested for this worker.</div>
      </div>
    </div>
  `,
  styles: [`
    .number { font-variant-numeric: tabular-nums; white-space: nowrap; }
    .status { display: inline-block; padding: 3px 8px; background: #e3eee9; color: #185847; border-radius: 3px; font-size: 11px; font-weight: 700; text-transform: capitalize; }
    .status.warn { background: #fff0ce; color: #7b5109; }
    .adjustment-bar { display: flex; align-items: center; justify-content: space-between; gap: 14px; padding: 12px 18px; }
    .adjustment-bar mat-form-field { min-width: 280px; }
    .adjustment-status { display: inline-flex; padding: 3px 7px; border: 1px solid #d6b262; border-radius: 3px; background: #fff1ca; color: #76510b; font-size: 10px; font-weight: 800; text-transform: capitalize; }
    .adjustment-status[data-status="approved"] { border-color: #8eb8a8; background: #e5f1eb; color: #185847; }
    .adjustment-status[data-status="rejected"] { border-color: #d89591; background: #f8dfde; color: #8c2929; }
    .review-strip { display: grid; grid-template-columns: 1.2fr 1.6fr auto; gap: 12px; align-items: start; margin-top: 10px; padding: 18px; background: #e8eeea; border: 1px solid var(--line); }
    .review-title { padding-top: 8px; } .review-title strong { display: block; font-size: 13px; }
    .review-strip .form-actions { display: flex; gap: 8px; padding-top: 4px; }
    .actions { white-space: nowrap; }
    tr.selected { background: #f2f6f3; }
    @media (max-width: 980px) { .review-strip { grid-template-columns: 1fr; } }
  `],
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class WorkersPage implements OnInit {
  readonly store = inject(WorkersStore);
  readonly adjustments = inject(AdjustmentsStore);
  readonly auth = useAuth();
  private readonly builder = new FormBuilder().nonNullable;
  readonly formOpen = signal(false);
  readonly adjustmentFormOpen = signal(false);
  readonly reviewTarget = signal<LimitAdjustment | null>(null);
  readonly saving = signal(false);
  readonly error = signal('');
  readonly form = this.builder.group({
    worker_code: ['RP-', [Validators.required, Validators.minLength(2)]],
    display_name: ['', [Validators.required, Validators.minLength(2)]],
    authorization_level: ['Controlled area L2', Validators.required],
    administrative_limit_msv: [12, [Validators.required, Validators.min(0.001)]],
    annual_limit_msv: [20, [Validators.required, Validators.min(0.001)]],
    profile_status: ['active' as ProfileStatus, Validators.required],
    period_start: [`${new Date().getFullYear()}-01-01`, Validators.required],
  });
  readonly adjustmentForm = this.builder.group({
    effective_from: ['', Validators.required],
    effective_to: ['', Validators.required],
    adjusted_limit_msv: [0, [Validators.required, Validators.min(0.001)]],
    reason: ['', [Validators.required, Validators.minLength(3), Validators.maxLength(500)]],
  });
  readonly reviewForm = this.builder.group({ note: ['', Validators.maxLength(500)] });

  constructor() {
    effect(() => {
      const workers = this.store.workers();
      if (workers.length && !this.adjustments.workerId()) {
        this.adjustments.load(workers[0].id);
      }
    });
  }

  ngOnInit(): void { this.store.load(); }
  get activeCount(): number { return this.store.workers().filter(item => item.profile_status === 'active').length; }
  get totalDose(): number { return this.store.workers().reduce((sum, item) => sum + item.period_dose_msv, 0); }

  create(): void {
    if (this.form.invalid) return;
    const value = this.form.getRawValue();
    const input: WorkerInput = { ...value, period_start: new Date(`${value.period_start}T00:00:00Z`).toISOString() };
    this.saving.set(true); this.error.set('');
    this.store.create(input).pipe(finalize(() => this.saving.set(false))).subscribe({
      next: () => { this.formOpen.set(false); this.form.reset({ ...value, worker_code: 'RP-', display_name: '' }); },
      error: error => this.error.set(apiErrorMessage(error)),
    });
  }

  selectWorker(workerId: number): void {
    this.reviewTarget.set(null);
    this.adjustments.load(workerId);
  }

  requestAdjustment(): void {
    if (this.adjustmentForm.invalid) return;
    const value = this.adjustmentForm.getRawValue();
    const input: LimitAdjustmentInput = {
      effective_from: new Date(`${value.effective_from}T00:00:00Z`).toISOString(),
      effective_to: new Date(`${value.effective_to}T00:00:00Z`).toISOString(),
      adjusted_limit_msv: value.adjusted_limit_msv,
      reason: value.reason,
    };
    this.saving.set(true); this.error.set('');
    this.adjustments.create(input).pipe(finalize(() => this.saving.set(false))).subscribe({
      next: () => { this.adjustmentFormOpen.set(false); this.adjustmentForm.reset({ adjusted_limit_msv: 0 }); },
      error: error => this.error.set(apiErrorMessage(error)),
    });
  }

  openReview(item: LimitAdjustment): void {
    this.reviewTarget.set(item);
    this.reviewForm.reset();
  }

  review(decision: 'approve' | 'reject'): void {
    const target = this.reviewTarget();
    if (!target) return;
    const note = this.reviewForm.controls.note.value.trim();
    if (decision === 'reject' && !note) return;
    this.saving.set(true); this.error.set('');
    this.adjustments.review(target.id, decision, note).pipe(finalize(() => this.saving.set(false))).subscribe({
      next: () => this.reviewTarget.set(null),
      error: error => this.error.set(apiErrorMessage(error)),
    });
  }
}
