export interface PortalRule {
  domain: string;
  text: string;
}

interface SourceLine {
  text: string;
  start: number;
  end: number;
}

interface Heading {
  level: number;
  text: string;
}

interface SourceRule extends PortalRule {
  start: number;
  bodyStart: number;
  end: number;
}

interface SourceSection {
  end: number;
  rules: SourceRule[];
}

interface Fence {
  marker: '`' | '~';
  length: number;
}

const portalRulesHeading = 'portal rules';

function splitSourceLines(content: string): SourceLine[] {
  const lines: SourceLine[] = [];
  let start = 0;

  while (start < content.length) {
    let textEnd = start;
    while (textEnd < content.length && content[textEnd] !== '\r' && content[textEnd] !== '\n') {
      textEnd += 1;
    }

    let end = textEnd;
    if (end < content.length) {
      end += content[end] === '\r' && content[end + 1] === '\n' ? 2 : 1;
    }
    lines.push({ text: content.slice(start, textEnd), start, end });
    start = end;
  }

  return lines;
}

function parseHeading(line: string): Heading | null {
  const match = /^ {0,3}(#{1,6})(?:[\t ]+(.*?)|[\t ]*)$/.exec(line);
  if (!match) {
    return null;
  }

  const text = (match[2] ?? '').replace(/[\t ]+#+[\t ]*$/, '').trim();
  return { level: match[1].length, text };
}

function parseOpeningFence(line: string): Fence | null {
  const match = /^ {0,3}(`{3,}|~{3,})/.exec(line);
  if (!match) {
    return null;
  }
  return { marker: match[1][0] as Fence['marker'], length: match[1].length };
}

function isClosingFence(line: string, fence: Fence): boolean {
  const match = /^ {0,3}(`+|~+)[\t ]*$/.exec(line);
  return match !== null && match[1][0] === fence.marker && match[1].length >= fence.length;
}

/** Locate only the canonical ## Portal Rules section. Other Context headings
 * remain opaque text; ATX-looking lines inside fenced code are ignored. */
function findPortalRulesSection(content: string): SourceSection | null {
  const lines = splitSourceLines(content);
  let section: SourceSection | null = null;
  let activeRule: SourceRule | null = null;
  let fence: Fence | null = null;

  function finishRule(end: number) {
    if (!section || !activeRule) {
      return;
    }
    activeRule.end = end;
    activeRule.text = content.slice(activeRule.bodyStart, end).trim();
    section.rules.push(activeRule);
    activeRule = null;
  }

  for (const line of lines) {
    if (fence) {
      if (isClosingFence(line.text, fence)) {
        fence = null;
      }
      continue;
    }

    const openingFence = parseOpeningFence(line.text);
    if (openingFence) {
      fence = openingFence;
      continue;
    }

    const heading = parseHeading(line.text);
    if (!section) {
      if (heading?.level === 2 && heading.text.toLowerCase() === portalRulesHeading) {
        section = { end: content.length, rules: [] };
      }
      continue;
    }

    if (heading && heading.level <= 2) {
      finishRule(line.start);
      section.end = line.start;
      break;
    }

    if (heading?.level === 3) {
      finishRule(line.start);
      if (heading.text) {
        activeRule = {
          domain: heading.text,
          text: '',
          start: line.start,
          bodyStart: line.end,
          end: content.length,
        };
      }
    }
  }

  finishRule(section?.end ?? content.length);
  return section;
}

/** A read-only projection of the UI-owned Markdown convention (ADR 0003).
 * No rule data is persisted outside the source Context File. */
export function parsePortalRules(content: string): PortalRule[] {
  return (findPortalRulesSection(content)?.rules ?? []).map(({ domain, text }) => ({
    domain,
    text,
  }));
}

function renderRule(domain: string, text: string, newline: string): string {
  const heading = `### ${domain.trim()}`;
  const trimmedText = text.trim();
  if (!trimmedText) {
    return heading;
  }
  return `${heading}${newline}${newline}${trimmedText.replace(/\r\n|\r|\n/g, newline)}`;
}

function separatorBeforeBlock(before: string, newline: string): string {
  if (/(?:\r\n|\r|\n){2}$/.test(before)) {
    return '';
  }
  return /(?:\r\n|\r|\n)$/.test(before) ? newline : newline + newline;
}

/** Add or replace one domain rule while leaving all Markdown before and after
 * that rule untouched. originalDomain identifies the rule during a rename;
 * for an add it is null. An existing domain match is always replaced, never
 * duplicated. */
export function upsertPortalRule(
  content: string,
  originalDomain: string | null,
  domain: string,
  text: string,
): string {
  const newline = /\r\n|\r|\n/.exec(content)?.[0] ?? '\n';
  const renderedRule = renderRule(domain, text, newline);
  const section = findPortalRulesSection(content);
  const targetDomain = (originalDomain ?? domain).trim().toLowerCase();
  const existingRule = section?.rules.find(
    (rule) => rule.domain.trim().toLowerCase() === targetDomain,
  );

  if (existingRule) {
    const before = content.slice(0, existingRule.start);
    const after = content.slice(existingRule.end);
    return before + renderedRule + (after ? newline + newline : newline) + after;
  }

  if (section) {
    const before = content.slice(0, section.end);
    const after = content.slice(section.end);
    return (
      before +
      separatorBeforeBlock(before, newline) +
      renderedRule +
      (after ? newline + newline : newline) +
      after
    );
  }

  const renderedSection = `## Portal Rules${newline}${newline}${renderedRule}${newline}`;
  if (!content) {
    return renderedSection;
  }
  return content + separatorBeforeBlock(content, newline) + renderedSection;
}
