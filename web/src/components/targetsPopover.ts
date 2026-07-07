// components/targetsPopover.ts — dashboard "Targets" quick-edit popover.
// The ONE edit that stays on the dashboard: a small anchored popover (not a
// page, not a modal) with a "Load from group" dropdown listing the tracker's
// def groups plus "— manual —". Apply PUTs target_group + the chosen group's
// BASE requirements mapped to target keys (same mapping as the settings
// form); "— manual —" only clears target_group and keeps target values.
import type { Tracker, TrackerGroupMap } from '../types';
import * as api from '../api';
import { esc } from '../utils/format';
import { groupRequirementsToTargets } from '../utils/group';
import { statsCache, strOf } from '../state';
import type { ToastType } from './toast';

interface PopoverDeps {
  trackers: () => Tracker[];
  groupDefs: () => TrackerGroupMap;
  loadTrackers: () => Promise<void>;
  toast: (msg: string, type?: ToastType) => void;
}

let _deps: PopoverDeps | null = null;
let _el: HTMLDivElement | null = null;
let _trackerId = '';

export function initTargetsPopover(deps: PopoverDeps): void {
  _deps = deps;
}

function ensureEl(): HTMLDivElement {
  if (_el) return _el;
  const el = document.createElement('div');
  el.id = 'targets-popover';
  el.className = 'targets-popover';
  el.style.display = 'none';
  el.addEventListener('mousedown', e => e.stopPropagation());
  el.addEventListener('click', e => e.stopPropagation());
  document.body.appendChild(el);

  // Close on outside click / Esc
  document.addEventListener('mousedown', e => {
    if (_el && _el.style.display !== 'none' && !_el.contains(e.target as Node)) closeTargetsPopover();
  });
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') closeTargetsPopover();
  });
  _el = el;
  return el;
}

/** Open the popover anchored to the pencil button that was clicked. */
export function openTargetsPopover(trackerId: string, anchor: HTMLElement): void {
  if (!_deps) return;
  const t = _deps.trackers().find(x => x.id === trackerId);
  if (!t) return;
  const groups = _deps.groupDefs()[t.def_key] ?? [];
  if (!groups.length) return; // pencil shouldn't render, but be safe

  const el = ensureEl();
  _trackerId = trackerId;

  // Live group from merged stats — highlighted so the user can see at a
  // glance where they currently sit when choosing a target group.
  const liveGroup = strOf(statsCache[trackerId], 'group');
  el.innerHTML = `
    <div class="targets-popover-title">Edit targets</div>
    <label class="form-label" style="font-size:11px" for="targets-popover-select">Load from group</label>
    <select class="form-input" id="targets-popover-select" style="margin:4px 0 10px">
      <option value="">&#8212; manual &#8212;</option>
      ${groups.map(g => {
        const isCurrent = !!liveGroup && g.name === liveGroup;
        const style = isCurrent ? ' style="color:var(--amber)"' : '';
        const label = isCurrent ? `★ ${esc(g.name)} (current)` : esc(g.name);
        return `<option value="${esc(g.name)}"${style} ${g.name === t.target_group ? 'selected' : ''}>${label}</option>`;
      }).join('')}
    </select>
    <div class="targets-popover-actions">
      <button type="button" class="btn btn-ghost btn-sm" id="targets-popover-cancel">Cancel</button>
      <button type="button" class="btn btn-primary btn-sm" id="targets-popover-apply">Apply</button>
    </div>`;

  el.querySelector('#targets-popover-cancel')?.addEventListener('click', closeTargetsPopover);
  el.querySelector('#targets-popover-apply')?.addEventListener('click', () => { void applyTargetsPopover(); });

  // Position near the anchor (fixed coords; clamp to viewport)
  el.style.display = 'block';
  el.style.visibility = 'hidden';
  const r = anchor.getBoundingClientRect();
  const w = el.offsetWidth, h = el.offsetHeight;
  let left = r.left;
  let top  = r.bottom + 6;
  if (left + w > window.innerWidth - 8)  left = Math.max(8, window.innerWidth - w - 8);
  if (top + h > window.innerHeight - 8)  top  = Math.max(8, r.top - h - 6);
  el.style.left = `${left}px`;
  el.style.top  = `${top}px`;
  el.style.visibility = '';
  (el.querySelector('#targets-popover-select') as HTMLSelectElement | null)?.focus();
}

export function closeTargetsPopover(): void {
  if (_el) _el.style.display = 'none';
  _trackerId = '';
}

async function applyTargetsPopover(): Promise<void> {
  if (!_deps || !_trackerId) return;
  const t = _deps.trackers().find(x => x.id === _trackerId);
  const sel = document.getElementById('targets-popover-select') as HTMLSelectElement | null;
  if (!t || !sel) return;
  const groupName = sel.value;

  let payload;
  if (!groupName) {
    // "— manual —": only clear target_group, keep the current target values.
    payload = { target_group: '' };
  } else {
    const g = (_deps.groupDefs()[t.def_key] ?? []).find(x => x.name === groupName);
    if (!g) return;
    payload = { target_group: groupName, targets: groupRequirementsToTargets(g.requirements) };
  }

  const btn = document.getElementById('targets-popover-apply') as HTMLButtonElement | null;
  if (btn) btn.disabled = true;
  const { ok } = await api.updateTracker(_trackerId, payload);
  if (btn) btn.disabled = false;
  if (ok) {
    closeTargetsPopover();
    _deps.toast(groupName ? `Targets loaded from ${groupName}` : 'Targets set to manual', 'success');
    await _deps.loadTrackers(); // re-renders the row/card with fresh targets
  } else {
    _deps.toast('Failed to update targets', 'error');
  }
}
