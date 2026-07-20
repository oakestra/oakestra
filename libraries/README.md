### How to use it:
To install a non-published package, add it in the `requirements.txt` file as follows:

```py
git+https://github.com/{username}/{project}.git@{branch}#subdirectory=libraries/{library_name}
```
For development purposes use:
```py
pip install -e .
```

## Available libraries

- `oakestra_utils_library` contains shared status and scheduling types.
- `resource_abstractor_client` contains the Resource Abstractor HTTP client.
- `oakestra_logging` defines the versioned structured JSON logging contract used by Python
  orchestrator services. See its [README](./oakestra_logging/README.md) for API and schema details.
