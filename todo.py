#!/usr/bin/env python3

import argparse
import os
import re
import sys
from dataclasses import dataclass
from itertools import chain
from multiprocessing import Pool
from pathlib import Path
from typing import List, Tuple, Optional

DESCRIPTION = """
Look for TODOs in files.

@FIXME - Issue that needs to be fixed.
@HACK  - A hack that needs to be replaced.
@TEMP  - Temporary solution that needs to be replaced.
@TODO  - Action item that needs to be done.
@XXX   - A note to the reader.
"""


def main(args):

    repo = current_git_repo()
    paths = [repo] if repo else todo_paths()

    with Pool() as pool:
        files = pool.map(find_files, paths)
        results = [
            res for res
            in pool.map(search_file, chain(*files))
            if res and res.todos
        ]

    for res in results:
        print(res)


def find_files(directory: Path) -> List[Path]:
    """Recursively finds files in a directory."""
    res = list()

    def search(p: Path):
        for item in p.iterdir():
            if item.is_dir():
                search(item)
            elif item.is_file():
                res.append(item)

    search(directory)
    return res


@dataclass
class SearchResult:
    file: Path
    todos: List[Tuple[int, str, str]]  # (line, tag, text)

    def __str__(self):
        name = str(self.file.relative_to(Path.home()))
        return '\n'.join([
            f'{tag:<7} {name}:{line:<4} {text}'
            for line, tag, text in self.todos
        ])


def search_file(path: Path) -> Optional[SearchResult]:
    """Searches a file for TODOs."""
    todos = list()
    regex = re.compile(r'^\s+(//|#|/?\*)\s+(?P<tag>@[A-Z]+) (?P<text>.*)')

    with open(path, 'r') as f:
        try:
            for line, text in enumerate(f, 1):
                m = regex.search(text)
                if m:
                    todos.append((line, m['tag'], m['text'].strip()))
        except UnicodeDecodeError:
            return None

    return SearchResult(path, todos)


# ------------------------------------------------------------------------------
# Helpers

def bail(msg: str) -> None:
    """Prints an error message and exits."""
    print(f'ERROR: {msg}', file=sys.stderr)
    exit(1)


def todo_paths() -> List[Path]:
    """Returns a list of paths to TODO files."""
    if 'TODO_PATH' not in os.environ:
        bail("TODO_PATH not set.")
    return [
        Path(path)
        for path in os.environ['TODO_PATH'].split(':')
        if os.path.exists(path)
    ]


def current_git_repo() -> Optional[Path]:
    """Returns the path to the current git repo."""
    cwd = Path.cwd()
    for path in [cwd, *cwd.parents]:
        if (path / '.git').exists():
            return path


# ------------------------------------------------------------------------------

if __name__ == '__main__':
    CLI = argparse.ArgumentParser(
        description=DESCRIPTION,
        formatter_class=argparse.RawTextHelpFormatter,
    )
    CLI.add_argument('-v', dest='verbose', action='store_true',
                     help='Verbose output.')
    main(CLI.parse_args())
