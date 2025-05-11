In case if you want to test how GitHub Actions works:
```bash
act -j build -P ubuntu-latest=ghcr.io/catthehacker/ubuntu:act-latest -e .github/tag-event.json
```