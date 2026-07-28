import { AppTheme } from '../common/Routes';

/** Soft Markdown block (Android Components.splitMarkdownBlocks parity). */
export interface MdBlock {
  kind: string;
  content: string;
  /** Ordered-list number prefix when kind === 'ol' */
  prefix: string;
}

/** Inline segment for bold / code / plain. */
export interface MdInlineSeg {
  kind: string;
  text: string;
}

/**
 * Split Markdown into block tuples — headings, fences, lists, hr, text.
 * Re-run on each stream chunk (caller throttles UI).
 */
export function splitMarkdownBlocks(text: string): MdBlock[] {
  const blocks: MdBlock[] = [];
  const lines: string[] = (text || '').split('\n');
  let i: number = 0;
  while (i < lines.length) {
    const line: string = lines[i];
    const trimmed: string = line.replace(/^\s+/, '');
    if (trimmed.indexOf('```') === 0) {
      const codeLines: string[] = [];
      i++;
      while (i < lines.length && lines[i].replace(/^\s+/, '').indexOf('```') !== 0) {
        codeLines.push(lines[i]);
        i++;
      }
      if (i < lines.length) {
        i++;
      }
      blocks.push({ kind: 'code', content: codeLines.join('\n'), prefix: '' });
      continue;
    }
    if (/^[-*_]{3,}\s*$/.test(trimmed)) {
      blocks.push({ kind: 'hr', content: '', prefix: '' });
      i++;
      continue;
    }
    if (trimmed.indexOf('### ') === 0) {
      blocks.push({ kind: 'h3', content: trimmed.substring(4).trim(), prefix: '' });
      i++;
      continue;
    }
    if (trimmed.indexOf('## ') === 0) {
      blocks.push({ kind: 'h2', content: trimmed.substring(3).trim(), prefix: '' });
      i++;
      continue;
    }
    if (trimmed.indexOf('# ') === 0) {
      blocks.push({ kind: 'h1', content: trimmed.substring(2).trim(), prefix: '' });
      i++;
      continue;
    }
    if (trimmed.indexOf('- ') === 0 || trimmed.indexOf('* ') === 0) {
      blocks.push({ kind: 'ul', content: trimmed.substring(2), prefix: '' });
      i++;
      continue;
    }
    const olMatch = trimmed.match(/^(\d+)\.\s+(.*)$/);
    if (olMatch && olMatch.length >= 3) {
      blocks.push({ kind: 'ol', content: olMatch[2], prefix: olMatch[1] });
      i++;
      continue;
    }
    const normal: string[] = [];
    while (i < lines.length) {
      const l: string = lines[i];
      const t: string = l.replace(/^\s+/, '');
      if (t.indexOf('```') === 0 || /^[-*_]{3,}\s*$/.test(t) ||
        t.indexOf('# ') === 0 || t.indexOf('## ') === 0 || t.indexOf('### ') === 0 ||
        t.indexOf('- ') === 0 || t.indexOf('* ') === 0 || /^\d+\.\s+/.test(t)) {
        break;
      }
      normal.push(l);
      i++;
    }
    if (normal.length > 0) {
      blocks.push({ kind: 'text', content: normal.join('\n'), prefix: '' });
    }
  }
  return blocks;
}

/** Inline **bold**, `code`, *italic* — soft subset of Android buildInlineMarkdown. */
export function parseInlineSegments(raw: string): MdInlineSeg[] {
  const out: MdInlineSeg[] = [];
  const text: string = raw || '';
  let i: number = 0;
  while (i < text.length) {
    if (i + 1 < text.length && text.charAt(i) === '*' && text.charAt(i + 1) === '*') {
      const end: number = text.indexOf('**', i + 2);
      if (end > i) {
        out.push({ kind: 'bold', text: text.substring(i + 2, end) });
        i = end + 2;
        continue;
      }
    }
    if (text.charAt(i) === '`') {
      const end: number = text.indexOf('`', i + 1);
      if (end > i) {
        out.push({ kind: 'code', text: text.substring(i + 1, end) });
        i = end + 1;
        continue;
      }
    }
    if (text.charAt(i) === '*' && (i + 1 >= text.length || text.charAt(i + 1) !== '*')) {
      const end: number = text.indexOf('*', i + 1);
      if (end > i && (end + 1 >= text.length || text.charAt(end + 1) !== '*')) {
        out.push({ kind: 'italic', text: text.substring(i + 1, end) });
        i = end + 1;
        continue;
      }
    }
    let j: number = i + 1;
    while (j < text.length) {
      const c: string = text.charAt(j);
      if (c === '*' || c === '`') {
        break;
      }
      j++;
    }
    out.push({ kind: 'text', text: text.substring(i, j) });
    i = j;
  }
  if (out.length === 0) {
    out.push({ kind: 'text', text: text });
  }
  return out;
}

/**
 * Soft Markdown renderer for AI assistant bubbles.
 * Re-parses on each @Prop content change (stream-friendly).
 */
@Component
export struct MarkdownText {
  @StorageLink('aiops_elevated') tElevated: string = '#192130';
  @StorageLink('aiops_border_soft') tBorderSoft: string = '#222C3D';
  @StorageLink('aiops_text') tText: string = '#E6E8F0';

  @Prop content: string = '';
  @Prop color: string = '';
  @Prop fontSize: number = 14;

  private blocks(): MdBlock[] {
    return splitMarkdownBlocks(this.content || '');
  }

  private textColor(): string {
    return this.color.length > 0 ? this.color : this.tText;
  }

  @Builder
  inlineSpans(raw: string, size: number, weight: FontWeight) {
    ForEach(parseInlineSegments(raw), (s: MdInlineSeg, idx: number) => {
      if (s.kind === 'bold') {
        Span(s.text)
          .fontWeight(FontWeight.Bold)
          .fontColor(this.textColor())
          .fontSize(size)
      } else if (s.kind === 'italic') {
        Span(s.text)
          .fontStyle(FontStyle.Italic)
          .fontColor(this.textColor())
          .fontSize(size)
      } else if (s.kind === 'code') {
        Span(s.text)
          .fontFamily('monospace')
          .fontColor(this.textColor())
          .fontSize(Math.max(10, size - 1))
      } else {
        Span(s.text)
          .fontWeight(weight)
          .fontColor(this.textColor())
          .fontSize(size)
      }
    }, (s: MdInlineSeg, idx: number) => `in-${idx}-${s.kind}-${s.text.length}`)
  }

  build() {
    Column({ space: 2 }) {
      ForEach(this.blocks(), (b: MdBlock, bi: number) => {
        if (b.kind === 'code') {
          Text(b.content)
            .fontFamily('monospace')
            .fontSize(Math.max(10, this.fontSize - 1))
            .fontColor(this.textColor())
            .width('100%')
            .padding({ left: 10, right: 10, top: 8, bottom: 8 })
            .backgroundColor(this.tElevated)
            .borderRadius(6)
            .border({ width: 1, color: this.tBorderSoft })
        } else if (b.kind === 'h1') {
          Text() {
            this.inlineSpans(b.content, this.fontSize + 5, FontWeight.Bold)
          }
          .width('100%')
        } else if (b.kind === 'h2') {
          Text() {
            this.inlineSpans(b.content, this.fontSize + 3, FontWeight.Bold)
          }
          .width('100%')
        } else if (b.kind === 'h3') {
          Text() {
            this.inlineSpans(b.content, this.fontSize + 1, FontWeight.Bold)
          }
          .width('100%')
        } else if (b.kind === 'hr') {
          Divider()
            .color(this.tBorderSoft)
            .strokeWidth(AppTheme.hairline)
            .margin({ top: 4, bottom: 4 })
        } else if (b.kind === 'ul') {
          Row() {
            Text('\u2022 ')
              .fontSize(this.fontSize)
              .fontColor(this.textColor())
            Text() {
              this.inlineSpans(b.content, this.fontSize, FontWeight.Regular)
            }
            .layoutWeight(1)
          }
          .width('100%')
          .padding({ left: 12 })
          .alignItems(VerticalAlign.Top)
        } else if (b.kind === 'ol') {
          Row() {
            Text(`${b.prefix.length > 0 ? b.prefix : '•'}. `)
              .fontSize(this.fontSize)
              .fontWeight(FontWeight.Medium)
              .fontColor(this.textColor())
            Text() {
              this.inlineSpans(b.content, this.fontSize, FontWeight.Regular)
            }
            .layoutWeight(1)
          }
          .width('100%')
          .padding({ left: 12 })
          .alignItems(VerticalAlign.Top)
        } else {
          Text() {
            this.inlineSpans(b.content, this.fontSize, FontWeight.Regular)
          }
          .width('100%')
        }
      }, (b: MdBlock, bi: number) => `md-${bi}-${b.kind}-${b.content.length}-${b.prefix}`)
    }
    .width('100%')
    .alignItems(HorizontalAlign.Start)
  }
}
