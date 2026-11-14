#!/usr/bin/env python3
"""Generate a crisp 1024x1024 SYNTRIX macOS AppIcon.png using numpy + zlib."""
import os
import struct
import zlib
import numpy as np


def write_png(path: str, rgba: np.ndarray) -> None:
    h, w, _ = rgba.shape
    raw = b"".join(b"\x00" + rgba[y].tobytes() for y in range(h))

    def chunk(tag: bytes, data: bytes) -> bytes:
        return struct.pack("!I", len(data)) + tag + data + struct.pack("!I", zlib.crc32(tag + data) & 0xFFFFFFFF)

    ihdr = struct.pack("!IIBBBBB", w, h, 8, 6, 0, 0, 0)
    png = b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", ihdr) + chunk(b"IDAT", zlib.compress(raw, 6)) + chunk(b"IEND", b"")
    with open(path, "wb") as f:
        f.write(png)


def main() -> None:
    size = 1024
    y, x = np.mgrid[0:size, 0:size].astype(np.float32)
    cx = cy = size / 2.0
    dx = (x - cx) / (size / 2.0)
    dy = (y - cy) / (size / 2.0)
    r = np.sqrt(dx * dx + dy * dy)
    theta = np.arctan2(dy, dx)

    # Rounded square macOS squircle mask
    ax = np.abs(dx)
    ay = np.abs(dy)
    corner_r = 0.22
    box_dist = np.maximum(ax - (0.92 - corner_r), 0.0) ** 2 + np.maximum(ay - (0.92 - corner_r), 0.0) ** 2
    inside = (ax <= 0.92) & (ay <= 0.92) & (box_dist <= corner_r ** 2)

    img = np.zeros((size, size, 4), dtype=np.float32)
    # Dark HUD navy background gradient
    bg_r = 4 + 14 * (1.0 - np.clip(r, 0, 1))
    bg_g = 12 + 38 * (1.0 - np.clip(r, 0, 1))
    bg_b = 24 + 62 * (1.0 - np.clip(r, 0, 1))

    img[:, :, 0] = bg_r
    img[:, :, 1] = bg_g
    img[:, :, 2] = bg_b
    img[:, :, 3] = np.where(inside, 255.0, 0.0)

    # Subtle outer border
    border = inside & ((ax > 0.895) | (ay > 0.895) | (box_dist > (corner_r - 0.015) ** 2))
    img[border, 0] = 38
    img[border, 1] = 198
    img[border, 2] = 228

    # Glowing cyan HUD rings + segmented arcs + core
    ring1 = np.exp(-((r - 0.62) / 0.022) ** 2)
    ring2 = np.exp(-((r - 0.44) / 0.018) ** 2) * (0.4 + 0.6 * (np.sin(theta * 6.0) > -0.2))
    ring3 = np.exp(-((r - 0.26) / 0.016) ** 2)
    core = np.exp(-(r / 0.14) ** 2)
    glow = 0.25 * np.exp(-(r / 0.55) ** 2)

    cyan = np.array([38.0, 210.0, 240.0], dtype=np.float32)
    intensity = np.clip(ring1 + ring2 + ring3 + core + glow, 0.0, 1.3)
    for c in range(3):
        img[:, :, c] = np.clip(img[:, :, c] + cyan[c] * intensity, 0, 255)

    out_dir = os.path.join(os.path.dirname(__file__), "..", "app", "Resources")
    os.makedirs(out_dir, exist_ok=True)
    out_png = os.path.join(out_dir, "AppIcon_1024.png")
    write_png(out_png, img.astype(np.uint8))
    print("Wrote", out_png)


if __name__ == "__main__":
    main()
