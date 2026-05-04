#!/usr/bin/env python3
"""Run Gemini CLI in headless mode for visual interpretation tasks."""

from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
from pathlib import Path


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Ask Gemini CLI to interpret one or more visual files."
    )
    parser.add_argument(
        "--image",
        action="append",
        required=True,
        help="Path to an image or Gemini-supported visual file. Repeat for comparisons.",
    )
    parser.add_argument(
        "--prompt",
        required=True,
        help="The visual interpretation task to ask Gemini to perform.",
    )
    parser.add_argument(
        "--model",
        default="pro",
        help='Gemini model or alias to use. Defaults to "pro".',
    )
    parser.add_argument(
        "--timeout",
        type=int,
        default=240,
        help="Seconds to wait before aborting the Gemini command.",
    )
    parser.add_argument(
        "--retries",
        type=int,
        default=2,
        help="Number of additional attempts when Gemini returns an empty or malformed answer.",
    )
    parser.add_argument(
        "--raw-json",
        action="store_true",
        help="Print Gemini CLI's full JSON response instead of only the response text.",
    )
    return parser.parse_args()


def resolve_visual_files(paths: list[str]) -> list[Path]:
    resolved: list[Path] = []
    for raw_path in paths:
        path = Path(raw_path).expanduser().resolve()
        if not path.exists():
            raise FileNotFoundError(f"visual file does not exist: {path}")
        if not path.is_file():
            raise ValueError(f"visual path is not a file: {path}")
        if "{" in str(path) or "}" in str(path):
            raise ValueError(f"visual path cannot contain braces for Gemini injection: {path}")
        resolved.append(path)
    return resolved


def build_prompt(files: list[Path], task: str) -> str:
    file_lines = [f"Visual file {index}: @{{{path}}}" for index, path in enumerate(files, 1)]
    return "\n".join(
        [
            "Use the attached visual file(s) to answer the task.",
            "If a detail is not visible or is uncertain, say so instead of guessing.",
            'Return valid compact JSON only, with exactly this schema: {"answer":"..."}',
            "Do not add markdown, preamble, role labels, or extra keys.",
            "",
            *file_lines,
            "",
            f"Task: {task}",
        ]
    )


def gemini_command(files: list[Path], prompt: str, model: str) -> list[str]:
    include_dirs = sorted({str(path.parent) for path in files})
    command = [
        "gemini",
        "-p",
        prompt,
        "--model",
        model,
        "--output-format",
        "json",
        "--approval-mode",
        "plan",
        "--skip-trust",
    ]
    for directory in include_dirs:
        command.extend(["--include-directories", directory])
    return command


def clean_response(text: str) -> str:
    cleaned = text.strip()

    cleaned = re.sub(r"(?s)<EPHEMERAL_MESSAGE>.*?</EPHEMERAL_MESSAGE>", "", cleaned).strip()
    ephemeral_intro = "The following is an ephemeral message not actually sent by the user."
    if ephemeral_intro in cleaned:
        cleaned = cleaned.split(ephemeral_intro, 1)[0].strip()

    # Gemini CLI/model responses may prefix a role-like token on its own line.
    while True:
        first_line, separator, rest = cleaned.partition("\n")
        if not separator:
            break
        if re.fullmatch(r"[A-Za-z_]{2,40}", first_line.strip()):
            cleaned = rest.strip()
            continue
        break

    # Some Gemini CLI/model combinations emit a lone leading "." before text.
    if len(cleaned) > 1 and cleaned.startswith(".") and (cleaned[1].isspace() or cleaned[1].isupper()):
        cleaned = cleaned[1:].lstrip()
    return cleaned


def extract_answer(text: str) -> str:
    cleaned = clean_response(text)
    if cleaned.startswith("```"):
        cleaned = re.sub(r"^```(?:json)?\s*", "", cleaned)
        cleaned = re.sub(r"\s*```$", "", cleaned).strip()

    decoder = json.JSONDecoder()
    found_answer = None
    for match in re.finditer(r"\{", cleaned):
        try:
            candidate, _ = decoder.raw_decode(cleaned[match.start():])
        except json.JSONDecodeError:
            continue
        answer = candidate.get("answer") if isinstance(candidate, dict) else None
        if isinstance(answer, str):
            found_answer = clean_response(answer)
    if found_answer is not None:
        return found_answer

    try:
        payload = json.loads(cleaned)
    except json.JSONDecodeError:
        return cleaned

    answer = payload.get("answer") if isinstance(payload, dict) else None
    if isinstance(answer, str):
        return clean_response(answer)
    return cleaned


def is_bad_answer(answer: str) -> bool:
    stripped = answer.strip()
    if stripped in {"", "."}:
        return True
    if "<EPHEMERAL_MESSAGE>" in stripped:
        return True
    return False


def main() -> int:
    args = parse_args()
    if shutil.which("gemini") is None:
        print("error: gemini CLI was not found on PATH", file=sys.stderr)
        return 127

    try:
        files = resolve_visual_files(args.image)
        prompt = build_prompt(files, args.prompt)
        command = gemini_command(files, prompt, args.model)
    except (FileNotFoundError, ValueError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    last_error = ""
    for attempt in range(args.retries + 1):
        if attempt:
            retry_prompt = prompt + "\n\nPrevious response was unusable. Return ONLY valid JSON matching {\"answer\":\"...\"}."
            command = gemini_command(files, retry_prompt, args.model)

        try:
            result = subprocess.run(
                command,
                check=False,
                capture_output=True,
                text=True,
                timeout=args.timeout,
                cwd=os.getcwd(),
            )
        except subprocess.TimeoutExpired as exc:
            last_error = str(exc)
            continue

        if result.returncode != 0:
            last_error = f"gemini exited with code {result.returncode}"
            if result.stderr.strip():
                last_error += "\n" + result.stderr.strip()
            if result.stdout.strip():
                last_error += "\n" + result.stdout.strip()
            continue

        if args.raw_json:
            print(result.stdout.strip())
            return 0

        try:
            payload = json.loads(result.stdout)
        except json.JSONDecodeError:
            answer = clean_response(result.stdout)
            if not is_bad_answer(answer):
                print(answer)
                return 0
            last_error = "gemini returned malformed JSON from the CLI"
            continue

        error = payload.get("error")
        if error:
            last_error = f"gemini returned an error: {error}"
            continue

        response = payload.get("response")
        if not isinstance(response, str):
            answer = json.dumps(payload, indent=2)
        else:
            answer = extract_answer(response)

        if is_bad_answer(answer):
            last_error = f"gemini returned an unusable visual answer: {answer!r}"
            continue

        print(answer)
        return 0

    if last_error:
        print(f"error: {last_error}", file=sys.stderr)
    else:
        print("error: gemini did not return a usable visual answer", file=sys.stderr)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
