"""Draw the app icon: resources/icon.png, 1024x1024.

A graphite squircle in the macOS icon grid (824 px body, continuous
corners, soft shadow), a power ring that runs from the app's input green
to its output periwinkle, and a rounded bolt. Drawn at 4x and scaled down.

    debugger/.venv/bin/python tools/make-icon.py
"""

import math
import pathlib

from PIL import Image, ImageChops, ImageDraw, ImageFilter

ROOT = pathlib.Path(__file__).resolve().parent.parent
OUT = ROOT / "resources" / "icon.png"

SS = 4                 # supersampling
N = 1024 * SS
BODY = 824 * SS        # Apple's icon grid: 824 of 1024
RADIUS = 186 * SS      # corner radius of the 824 px body

# The app's tokens (internal/ui/theme.go).
TOP = (34, 37, 49)
BOTTOM = (14, 15, 20)
GREEN = (98, 214, 154)
BLUE = (147, 168, 255)
BOLT = (244, 245, 250)


def squircle_mask(size, body, radius):
    """The icon shape: a rounded square whose corners blend into the sides
    over a longer run than a circular arc, as Apple's continuous corners do."""
    m = Image.new("L", (size, size), 0)
    o = (size - body) / 2
    ImageDraw.Draw(m).rounded_rectangle((o, o, o + body, o + body), radius=radius, fill=255)
    # Soften the joint between arc and side, then cut back to a crisp edge.
    m = m.filter(ImageFilter.GaussianBlur(radius * 0.28))
    return m.point(lambda v: 255 if v >= 128 else 0)


def rounded(mask, r):
    """Round every corner of a shape by r: blur, then threshold."""
    return mask.filter(ImageFilter.GaussianBlur(r)).point(lambda v: 255 if v >= 128 else 0)


def vertical_gradient(size, top, bottom):
    g = Image.new("RGB", (1, size))
    for y in range(size):
        k = y / (size - 1)
        g.putpixel((0, y), tuple(round(top[i] + (bottom[i] - top[i]) * k) for i in range(3)))
    return g.resize((size, size))


def mix(a, b, k):
    return tuple(round(a[i] + (b[i] - a[i]) * k) for i in range(3))


def main():
    canvas = Image.new("RGBA", (N, N), (0, 0, 0, 0))
    mask = squircle_mask(N, BODY, RADIUS)

    # Shadow under the body.
    shadow = Image.new("RGBA", (N, N), (0, 0, 0, 0))
    sm = mask.filter(ImageFilter.GaussianBlur(14 * SS))
    shadow.putalpha(sm.point(lambda v: v * 0.45))
    canvas.alpha_composite(shadow, (0, 10 * SS))

    # Body: graphite gradient and a faint top sheen.
    body = vertical_gradient(N, TOP, BOTTOM).convert("RGBA")
    sheen = Image.new("L", (N, N), 0)
    ImageDraw.Draw(sheen).ellipse((-N * 0.3, -N * 1.1, N * 1.3, N * 0.5), fill=22)
    sheen = sheen.filter(ImageFilter.GaussianBlur(120 * SS))
    body = Image.composite(Image.new("RGBA", (N, N), (255, 255, 255, 255)), body, sheen)
    body.putalpha(mask)
    canvas.alpha_composite(body)

    # Hairline rim inside the edge.
    rim = ImageChops.subtract(mask, mask.filter(ImageFilter.MinFilter(2 * SS + 1)))
    rim_layer = Image.new("RGBA", (N, N), (255, 255, 255, 0))
    rim_layer.putalpha(rim.point(lambda v: v * 0.10))
    canvas.alpha_composite(rim_layer)

    # Power ring: a gauge open at the bottom, green to periwinkle.
    d = ImageDraw.Draw(canvas)
    cx, cy = N / 2, N / 2 + 6 * SS
    r = 262 * SS
    w = 74 * SS
    start, end = 125.0, 415.0  # degrees, clockwise from +x; 70° gap at the bottom
    steps = 900
    track = Image.new("RGBA", (N, N), (0, 0, 0, 0))
    ImageDraw.Draw(track).arc((cx - r - w / 2, cy - r - w / 2, cx + r + w / 2, cy + r + w / 2),
                              start, end, fill=(255, 255, 255, 18), width=w)
    canvas.alpha_composite(track)
    for i in range(steps):
        k = i / (steps - 1)
        a0 = start + (end - start) * i / steps
        a1 = start + (end - start) * (i + 1.6) / steps
        d.arc((cx - r - w / 2, cy - r - w / 2, cx + r + w / 2, cy + r + w / 2),
              a0, min(a1, end), fill=mix(GREEN, BLUE, k), width=w)
    for ang, col in ((start, GREEN), (end, BLUE)):
        t = math.radians(ang)
        ex, ey = cx + r * math.cos(t), cy + r * math.sin(t)
        d.ellipse((ex - w / 2, ey - w / 2, ex + w / 2, ey + w / 2), fill=col)

    # Bolt, with rounded corners from a thick round-joined outline.
    s = 2.55 * SS
    ox, oy = cx - 50 * s, cy - 64 * s
    bolt = [(58, 0), (14, 72), (46, 72), (36, 128), (86, 50), (53, 50), (66, 0)]
    pts = [(ox + x * s, oy + y * s) for x, y in bolt]
    glow = Image.new("RGBA", (N, N), (0, 0, 0, 0))
    gd = ImageDraw.Draw(glow)
    gd.polygon(pts, fill=(BLUE[0], BLUE[1], BLUE[2], 90))
    glow = glow.filter(ImageFilter.GaussianBlur(26 * SS))
    canvas.alpha_composite(glow)
    shape = Image.new("L", (N, N), 0)
    ImageDraw.Draw(shape).polygon(pts, fill=255)
    shape = rounded(shape.filter(ImageFilter.MaxFilter(9 * SS + 1)), 7 * SS)
    canvas.paste(Image.new("RGBA", (N, N), BOLT + (255,)), (0, 0), shape)

    canvas.resize((1024, 1024), Image.LANCZOS).save(OUT)
    print("wrote", OUT)


if __name__ == "__main__":
    main()
