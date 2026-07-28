#!/usr/bin/env python3
"""Mechanical AppTheme token unification for ArkTS UI files."""
from __future__ import annotations

import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1] / "entry" / "src" / "main" / "ets"

RADIUS_MAP = {
    4: "AppTheme.rChip",
    6: "AppTheme.rChip",
    8: "AppTheme.rXs",
    10: "AppTheme.rSm",
    11: "AppTheme.rSm",
    12: "AppTheme.rSm",
    14: "AppTheme.rMd",
    16: "AppTheme.rMd",
    18: "AppTheme.rMd",
    20: "AppTheme.rLg",
    22: "AppTheme.rLg",
    24: "AppTheme.rLg",
    25: "AppTheme.rLg",
    28: "AppTheme.rXl",
    30: "AppTheme.rXl",
}

FONT_MAP = {
    9: "AppTheme.fontTiny",
    10: "AppTheme.fontTiny",
    11: "AppTheme.fontMicro",
    12: "AppTheme.fontCaption",
    13: "AppTheme.fontSmall",
    14: "AppTheme.fontBody",
    15: "AppTheme.fontSubtitle",
    16: "AppTheme.fontLead",
    17: "AppTheme.fontTitle",
    18: "AppTheme.fontTitle",
    20: "AppTheme.fontTitle",
    22: "AppTheme.fontDisplay",
    28: "AppTheme.fontHero",
}

SKIP_DIRS = {"build", "oh_modules", "node_modules", ".hvigor"}


def ensure_import(text: str, rel: str) -> str:
    if re.search(r"import\s*\{[^}]*\bAppTheme\b", text):
        return text
    # depth: pages/X -> ../common/Routes ; components/X -> ../common/Routes
    parts = Path(rel).parts
    if parts[0] == "pages":
        imp = "import { AppTheme } from '../common/Routes';\n"
    elif parts[0] == "components":
        imp = "import { AppTheme } from '../common/Routes';\n"
    elif parts[0] == "common":
        return text
    else:
        imp = "import { AppTheme } from '../common/Routes';\n"
    # insert after last import block start
    m = list(re.finditer(r"^import\s+.+$", text, re.M))
    if not m:
        return imp + text
    last = m[-1]
    return text[: last.end()] + "\n" + imp + text[last.end() :]


def replace_radius(text: str) -> str:
    def one(m: re.Match[str]) -> str:
        n = int(m.group(1))
        tok = RADIUS_MAP.get(n)
        return f".borderRadius({tok})" if tok else m.group(0)

    text = re.sub(r"\.borderRadius\((\d+)\)", one, text)

    def corner(m: re.Match[str]) -> str:
        key, n = m.group(1), int(m.group(2))
        tok = RADIUS_MAP.get(n)
        return f"{key}: {tok}" if tok else m.group(0)

    text = re.sub(
        r"(topLeft|topRight|bottomLeft|bottomRight):\s*(\d+)",
        corner,
        text,
    )
    return text


def replace_font(text: str) -> str:
    def one(m: re.Match[str]) -> str:
        n = int(m.group(1))
        tok = FONT_MAP.get(n)
        return f".fontSize({tok})" if tok else m.group(0)

    return re.sub(r"\.fontSize\((\d+)\)", one, text)


def replace_common_pads(text: str) -> str:
    # only standalone padding(N) literals that match card/page pads
    text = re.sub(r"\.padding\(14\)", ".padding(AppTheme.cardPad)", text)
    text = re.sub(r"\.padding\(16\)", ".padding(AppTheme.pagePadH)", text)
    text = re.sub(r"\.padding\(24\)", ".padding(AppTheme.spaceXl)", text)
    text = re.sub(r"\.padding\(12\)", ".padding(AppTheme.spaceMd)", text)
    text = re.sub(r"\.padding\(8\)", ".padding(AppTheme.spaceSm)", text)
    return text


def process(path: Path, rel: str) -> bool:
    raw = path.read_text(encoding="utf-8")
    if "AppTheme" in path.name and path.name == "Routes.ets":
        return False
    orig = raw
    raw = replace_radius(raw)
    raw = replace_font(raw)
    raw = replace_common_pads(raw)
    if "AppTheme." in raw and raw != orig:
        raw = ensure_import(raw, rel)
    if raw != orig:
        path.write_text(raw, encoding="utf-8")
        return True
    return False


def main() -> None:
    changed = []
    for path in ROOT.rglob("*.ets"):
        if any(p in SKIP_DIRS for p in path.parts):
            continue
        rel = str(path.relative_to(ROOT)).replace("\\", "/")
        if rel == "common/Routes.ets":
            continue
        if process(path, rel):
            changed.append(rel)
    print(f"updated {len(changed)} files")
    for c in changed:
        print(c)


if __name__ == "__main__":
    main()
