import { describe, expect, it } from 'vitest';

import { formatDateTime, intervalLabel } from '@/utils/savedSearch';

describe('intervalLabel', () => {
  it('uses the common words for hourly, daily and weekly', () => {
    expect(intervalLabel(1)).toBe('stündlich');
    expect(intervalLabel(24)).toBe('täglich');
    expect(intervalLabel(168)).toBe('wöchentlich');
  });

  it('counts whole days and falls back to hours', () => {
    expect(intervalLabel(72)).toBe('alle 3 Tage');
    expect(intervalLabel(36)).toBe('alle 36 Stunden');
  });
});

describe('formatDateTime', () => {
  it('shows a dash for a missing timestamp', () => {
    expect(formatDateTime(undefined)).toBe('–');
  });
});
