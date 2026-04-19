import re

with open("internal/runner/materialize_test.go", "r") as f:
    content = f.read()

content = re.sub(
    r'"command": \[.*?\]',
    '"command": []string{"sh", "-c", "echo \'{}\' > \\"$AUTODEV_STAGE_RESULT_WORKING\\""}',
    content,
    flags=re.DOTALL
)

with open("internal/runner/materialize_test.go", "w") as f:
    f.write(content)
