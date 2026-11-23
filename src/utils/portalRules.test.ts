import { describe, expect, it } from 'vitest';

import { parsePortalRules, upsertPortalRule } from '@/utils/portalRules';

describe('parsePortalRules', () => {
  it('reads domain subsections without including the following Context section', () => {
    const content = [
      '# Anweisungen',
      '',
      '## Portal Rules',
      '',
      '### booli.se',
      'Folge dem Makler-Link.',
      '',
      '#### Details',
      'Auch Grundrisse laden.',
      '',
      '### hemnet.se',
      'Lies die vollständige Beschreibung.',
      '',
      '## Sonstiges',
      'Kein Teil einer Portal-Regel.',
      '',
    ].join('\n');

    expect(parsePortalRules(content)).toEqual([
      {
        domain: 'booli.se',
        text: 'Folge dem Makler-Link.\n\n#### Details\nAuch Grundrisse laden.',
      },
      { domain: 'hemnet.se', text: 'Lies die vollständige Beschreibung.' },
    ]);
  });

  it('ignores domain-looking headings inside fenced code', () => {
    const content = [
      '## Portal Rules',
      '',
      '```markdown',
      '### beispiel.example',
      'Nur ein Beispiel, keine Regel.',
      '```',
      '',
      '### booli.se',
      'Die echte Regel.',
      '',
    ].join('\n');

    expect(parsePortalRules(content)).toEqual([{ domain: 'booli.se', text: 'Die echte Regel.' }]);
  });

  it('returns no rules for free-form Context without the canonical section', () => {
    expect(parsePortalRules('## Portalregeln\n\n### booli.se\nFreier Text.')).toEqual([]);
  });
});

describe('upsertPortalRule', () => {
  it('adds the first rule as Markdown to an empty Context', () => {
    expect(upsertPortalRule('', null, 'booli.se', 'Folge dem Makler-Link.')).toBe(
      '## Portal Rules\n\n### booli.se\n\nFolge dem Makler-Link.\n',
    );
  });

  it('preserves existing non-Portal Markdown when adding a section', () => {
    expect(
      upsertPortalRule('## Bilder\n\nAlles lokal speichern.\n', null, 'booli.se', 'Neue Regel.'),
    ).toBe(
      '## Bilder\n\nAlles lokal speichern.\n\n## Portal Rules\n\n### booli.se\n\nNeue Regel.\n',
    );
  });

  it('adds a domain inside the existing section before subsequent Context text', () => {
    const content =
      '## Portal Rules\n\n### booli.se\n\nBooli-Regel.\n\n## Sonstiges\n\nBleibt erhalten.\n';

    expect(upsertPortalRule(content, null, 'hemnet.se', 'Hemnet-Regel.')).toBe(
      '## Portal Rules\n\n### booli.se\n\nBooli-Regel.\n\n### hemnet.se\n\nHemnet-Regel.\n\n## Sonstiges\n\nBleibt erhalten.\n',
    );
  });

  it('replaces only the selected domain and supports renaming it case-insensitively', () => {
    const content = [
      '# Anweisungen',
      '',
      '## Portal Rules',
      '',
      '### booli.se',
      'Alte Regel.',
      '',
      '### hemnet.se',
      'Unverändert.',
      '',
      '## Sonstiges',
      'Auch unverändert.',
      '',
    ].join('\n');

    expect(upsertPortalRule(content, 'BOOLI.SE', 'booli.se', 'Neue Regel.\nMit zwei Zeilen.')).toBe(
      [
        '# Anweisungen',
        '',
        '## Portal Rules',
        '',
        '### booli.se',
        '',
        'Neue Regel.',
        'Mit zwei Zeilen.',
        '',
        '### hemnet.se',
        'Unverändert.',
        '',
        '## Sonstiges',
        'Auch unverändert.',
        '',
      ].join('\n'),
    );
  });

  it('uses the existing CRLF line-ending convention when adding a rule', () => {
    expect(upsertPortalRule('## Bilder\r\n\r\nText', null, 'booli.se', 'Neue Regel.')).toBe(
      '## Bilder\r\n\r\nText\r\n\r\n## Portal Rules\r\n\r\n### booli.se\r\n\r\nNeue Regel.\r\n',
    );
  });
});
