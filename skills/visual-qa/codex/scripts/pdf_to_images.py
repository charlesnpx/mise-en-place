#!/usr/bin/env python3

"""Render a PDF into PNG page images for visual QA workflows."""

import argparse
import shutil
import subprocess
import sys
from pathlib import Path


def parse_args():
    parser = argparse.ArgumentParser(
        description="Render each page of a PDF to a PNG image.",
    )
    parser.add_argument("pdf", help="Path to the source PDF")
    parser.add_argument(
        "--output-dir",
        help="Directory for generated images. Defaults to <pdf-dir>/<pdf-stem>-pages",
    )
    parser.add_argument(
        "--dpi",
        type=int,
        default=200,
        help="Rasterization DPI. Defaults to 200.",
    )
    parser.add_argument(
        "--prefix",
        help="Output filename prefix. Defaults to the PDF stem.",
    )
    parser.add_argument(
        "--backend",
        choices=("auto", "pdftoppm", "pymupdf", "pdf2image"),
        default="auto",
        help="Rendering backend. Defaults to auto.",
    )
    return parser.parse_args()


def ensure_pdf(pdf_path):
    if not pdf_path.exists():
        raise FileNotFoundError(f"PDF not found: {pdf_path}")
    if not pdf_path.is_file():
        raise FileNotFoundError(f"PDF path is not a file: {pdf_path}")


def normalize_output_paths(paths, output_dir, prefix):
    normalized = []
    for index, source_path in enumerate(sorted(paths), start=1):
        target_path = output_dir / f"{prefix}_page_{index}.png"
        if Path(source_path) != target_path:
            shutil.move(str(source_path), str(target_path))
        normalized.append(target_path)
    return normalized


def render_with_pdftoppm(pdf_path, output_dir, dpi, prefix):
    executable = shutil.which("pdftoppm")
    if not executable:
        raise RuntimeError("pdftoppm is not installed or not on PATH.")

    temp_prefix = output_dir / f"{prefix}-render"
    subprocess.run(
        [
            executable,
            "-png",
            "-r",
            str(dpi),
            str(pdf_path),
            str(temp_prefix),
        ],
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    temp_paths = list(output_dir.glob(f"{temp_prefix.name}-*.png"))
    if not temp_paths:
        raise RuntimeError("pdftoppm did not produce any PNG files.")
    return normalize_output_paths(temp_paths, output_dir, prefix)


def render_with_pymupdf(pdf_path, output_dir, dpi, prefix):
    try:
        import fitz
    except ImportError as exc:
        raise RuntimeError("PyMuPDF is not installed.") from exc

    scale = dpi / 72
    matrix = fitz.Matrix(scale, scale)
    results = []
    with fitz.open(pdf_path) as document:
        for index, page in enumerate(document, start=1):
            target_path = output_dir / f"{prefix}_page_{index}.png"
            page.get_pixmap(matrix=matrix, alpha=False).save(target_path)
            results.append(target_path)
    if not results:
        raise RuntimeError("PyMuPDF did not produce any PNG files.")
    return results


def render_with_pdf2image(pdf_path, output_dir, dpi, prefix):
    try:
        from pdf2image import convert_from_path
    except ImportError as exc:
        raise RuntimeError("pdf2image is not installed.") from exc

    images = convert_from_path(str(pdf_path), dpi=dpi)
    results = []
    for index, image in enumerate(images, start=1):
        target_path = output_dir / f"{prefix}_page_{index}.png"
        image.save(target_path, "PNG")
        results.append(target_path)
    if not results:
        raise RuntimeError("pdf2image did not produce any PNG files.")
    return results


def render_pdf(pdf_path, output_dir, dpi, prefix, backend):
    backends = {
        "pdftoppm": render_with_pdftoppm,
        "pymupdf": render_with_pymupdf,
        "pdf2image": render_with_pdf2image,
    }
    order = ["pdftoppm", "pymupdf", "pdf2image"] if backend == "auto" else [backend]
    errors = []

    for name in order:
        try:
            return name, backends[name](pdf_path, output_dir, dpi, prefix)
        except Exception as exc:
            errors.append(f"{name}: {exc}")

    details = "\n".join(errors)
    raise RuntimeError(f"Failed to render PDF with available backends:\n{details}")


def main():
    args = parse_args()
    pdf_path = Path(args.pdf).expanduser().resolve()
    ensure_pdf(pdf_path)

    output_dir = (
        Path(args.output_dir).expanduser().resolve()
        if args.output_dir
        else pdf_path.parent / f"{pdf_path.stem}-pages"
    )
    output_dir.mkdir(parents=True, exist_ok=True)
    prefix = args.prefix or pdf_path.stem

    backend, images = render_pdf(pdf_path, output_dir, args.dpi, prefix, args.backend)
    print(f"backend={backend}")
    for image_path in images:
        print(image_path)


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(str(exc), file=sys.stderr)
        sys.exit(1)
