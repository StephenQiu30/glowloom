from __future__ import annotations

# Unicode 15.1 White_Space property, kept local so host Unicode updates cannot alter trimming.
WHITE_SPACE_RANGES = (
    (0x0009, 0x000D),
    (0x0020, 0x0020),
    (0x0085, 0x0085),
    (0x00A0, 0x00A0),
    (0x1680, 0x1680),
    (0x2000, 0x200A),
    (0x2028, 0x2029),
    (0x202F, 0x202F),
    (0x205F, 0x205F),
    (0x3000, 0x3000),
)

# Unicode 15.1 Scripts.txt entries whose Script property is Han.
HAN_RANGES = (
    (0x2E80, 0x2E99),
    (0x2E9B, 0x2EF3),
    (0x2F00, 0x2FD5),
    (0x3005, 0x3005),
    (0x3007, 0x3007),
    (0x3021, 0x3029),
    (0x3038, 0x303B),
    (0x3400, 0x4DBF),
    (0x4E00, 0x9FFF),
    (0xF900, 0xFA6D),
    (0xFA70, 0xFAD9),
    (0x16FE2, 0x16FE3),
    (0x16FF0, 0x16FF1),
    (0x20000, 0x2A6DF),
    (0x2A700, 0x2B739),
    (0x2B740, 0x2B81D),
    (0x2B820, 0x2CEA1),
    (0x2CEB0, 0x2EBE0),
    (0x2EBF0, 0x2EE5D),
    (0x2F800, 0x2FA1D),
    (0x30000, 0x3134A),
    (0x31350, 0x323AF),
)


def codepoint_in_ranges(codepoint: int, ranges: tuple[tuple[int, int], ...]) -> bool:
    return any(start <= codepoint <= end for start, end in ranges)


def strip_white_space(value: str) -> str:
    start = 0
    end = len(value)
    while start < end and codepoint_in_ranges(ord(value[start]), WHITE_SPACE_RANGES):
        start += 1
    while end > start and codepoint_in_ranges(ord(value[end - 1]), WHITE_SPACE_RANGES):
        end -= 1
    return value[start:end]


def is_han(character: str) -> bool:
    return codepoint_in_ranges(ord(character), HAN_RANGES)
