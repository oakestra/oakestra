## Customize run-a-cluster with specific versions

It's possible to use specific versions (tags or branches) by setting the `OAKESTRA_VERSION` environment variable when using the startup scripts.

### Using a specific tag (e.g., alpha version):

```bash
export OAKESTRA_VERSION=alpha-v0.4.403
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/main/scripts/StartOakestraFull.sh | sh -
```

The script will automatically generate an override file with all images tagged to the specified version.

### Using a branch for development:

```bash
export OAKESTRA_VERSION=develop
# Run from the repository directory to build images from source
cd /path/to/oakestra/repository
./scripts/StartOakestraFull.sh
```

When using a branch name, the script will build images from source if the repository directories are available.

### Using latest images (default):

```bash
# No OAKESTRA_VERSION set - uses latest images
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/main/scripts/StartOakestraFull.sh | sh -
```
 
