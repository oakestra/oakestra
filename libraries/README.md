### How to use it:
To install a non-published package, add it in the `requirements.txt` file as follows:

```py
git+https://github.com/{username}/{project}.git@{branch}#subdirectory=libraries/{library_name}
```
For development purposes use:
```py
pip install -e .
```
