import sys
import re

def replace_function(filepath, func_name, replacement_file):
    with open(filepath, 'r') as f:
        content = f.read()
    with open(replacement_file, 'r') as f:
        replacement = f.read()
    pattern = re.compile(
        r'^' + re.escape(func_name) + r'\(\) \{.*?^\}',
        re.MULTILINE | re.DOTALL
    )
    if not pattern.search(content):
        print(f"WARNING: function {func_name} not found in {filepath}", file=sys.stderr)
        sys.exit(1)
    new_content = pattern.sub(replacement.rstrip('\n'), content)
    with open(filepath, 'w') as f:
        f.write(new_content)

if __name__ == '__main__':
    if len(sys.argv) != 4:
        print(f"Usage: {sys.argv[0]} <file> <function_name> <replacement_file>", file=sys.stderr)
        sys.exit(1)
    replace_function(sys.argv[1], sys.argv[2], sys.argv[3])
