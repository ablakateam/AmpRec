"""Generate deterministic AmpRec application and recording icons."""

from pathlib import Path

from PIL import Image, ImageDraw


ROOT = Path(__file__).resolve().parents[1]
SIZES = (16, 24, 32, 48, 64, 128, 256)


def render(size: int, recording: bool) -> Image.Image:
    image = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    draw = ImageDraw.Draw(image)
    margin = max(1, round(size * 0.06))
    radius = round(size * 0.24)
    draw.rounded_rectangle(
        (margin, margin, size - margin - 1, size - margin - 1),
        radius=radius,
        fill=(24, 48, 59, 255),
    )
    heights = (0.25, 0.48, 0.72, 0.92, 0.62, 0.38, 0.57)
    center = size / 2
    spacing = size * 0.095
    line_width = max(1, round(size * 0.045))
    for index, factor in enumerate(heights):
        x = center + (index - 3) * spacing
        half = size * factor * 0.28
        draw.rounded_rectangle(
            (round(x - line_width / 2), round(center - half), round(x + line_width / 2), round(center + half)),
            radius=line_width,
            fill=(75, 219, 190, 255),
        )
    if recording:
        dot = round(size * 0.20)
        left = size - margin - dot
        top = margin
        draw.ellipse((left, top, left + dot, top + dot), fill=(255, 87, 108, 255))
    return image


def save_icon(path: Path, recording: bool) -> None:
    base = render(256, recording)
    base.save(path, format="ICO", sizes=[(size, size) for size in SIZES])


if __name__ == "__main__":
    save_icon(ROOT / "assets" / "AmpRec.ico", False)
    save_icon(ROOT / "assets" / "AmpRec-recording.ico", True)
