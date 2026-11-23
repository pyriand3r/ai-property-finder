import { describe, expect, it } from 'vitest';

import { statusColor, statusLabel, statusOptions } from '@/utils/listingStatus';

describe('listingStatus', () => {
  it('derives statusOptions from statusLabel, one option per status in declaration order', () => {
    expect(statusOptions).toEqual(
      Object.entries(statusLabel).map(([value, label]) => ({ value, label })),
    );
    expect(statusOptions[0]).toEqual({ value: 'new', label: 'Neu' });
  });

  it('defines a color for exactly the statuses that have a label', () => {
    expect(new Set(Object.keys(statusColor))).toEqual(new Set(Object.keys(statusLabel)));
  });

  it('gives every status a distinct marker color so Map View markers stay distinguishable', () => {
    expect(new Set(Object.values(statusColor)).size).toBe(Object.keys(statusColor).length);
  });
});
