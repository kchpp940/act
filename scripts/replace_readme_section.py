import sys

def replace_readme_section(filepath, snippet_file):
    with open(filepath, 'r') as f:
        content = f.read()
    with open(snippet_file, 'r') as f:
        snippet = f.read()
    start_marker = '<!-- AUTO-GENERATED from release.json'
    end_marker = '<!-- END AUTO-GENERATED -->'
    start_idx = content.find(start_marker)
    end_idx = content.find(end_marker)
    if start_idx != -1 and end_idx != -1:
        new_content = content[:start_idx] + snippet.rstrip('\n') + '\n' + end_marker + content[end_idx + len(end_marker):]
    else:
        anchor = '# Act User Guide'
        anchor_idx = content.find(anchor)
        if anchor_idx == -1:
            print(f"WARNING: Neither AUTO-GENERATED markers nor '{anchor}' found in {filepath}", file=sys.stderr)
            sys.exit(1)
        insert_pos = content.find('\n', anchor_idx) + 1
        after_guide = content.find('\n', content.find('act user guide', insert_pos))
        if after_guide == -1:
            after_guide = content.find('\n', insert_pos)
        new_content = content[:after_guide + 1] + '\n' + snippet.rstrip('\n') + '\n' + end_marker + '\n' + content[after_guide + 1:]
    with open(filepath, 'w') as f:
        f.write(new_content)

if __name__ == '__main__':
    if len(sys.argv) != 3:
        print(f"Usage: {sys.argv[0]} <readme_file> <snippet_file>", file=sys.stderr)
        sys.exit(1)
    replace_readme_section(sys.argv[1], sys.argv[2])
