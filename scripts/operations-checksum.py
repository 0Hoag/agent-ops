#!/usr/bin/env python3
import hashlib
import re
import sys

INSERT = re.compile(r"\s*(?:(?:--[^\n]*(?:\n|$))|(?:/\*.*?\*/\s*))*INSERT\s+INTO\b", re.IGNORECASE | re.DOTALL)

def statements(stream):
    text, result, start = stream.read(), [], 0
    single = double = False
    dollar = None
    i = 0
    while i < len(text):
        c = text[i]
        if dollar:
            if text.startswith(dollar, i):
                i += len(dollar); dollar = None; continue
        elif single:
            if c == "\\":
                i += 2; continue
            if c == "'":
                if text[i:i + 2] == "''": i += 2; continue
                single = False
        elif double:
            if c == '"':
                if text[i:i + 2] == '""': i += 2; continue
                double = False
        elif c == "'": single = True
        elif c == '"': double = True
        elif c == "$":
            match = re.match(r"\$[A-Za-z_][A-Za-z0-9_]*\$|\$\$", text[i:])
            if match: dollar = match.group(0); i += len(dollar); continue
        elif c == ";":
            statement = text[start:i + 1]
            match = INSERT.match(statement)
            if match:
                result.append(statement[re.search(r"INSERT\s+INTO", statement, re.IGNORECASE).start():])
            start = i + 1
        i += 1
    if start < len(text) and INSERT.match(text[start:]): result.append(text[start:])
    return result

digest = hashlib.sha256()
for statement in sorted(statements(sys.stdin)):
    digest.update(statement.encode())
print(digest.hexdigest())
