from setuptools import find_packages, setup

setup(
    name="resource_abstractor_client",
    version="0.1.0",
    packages=find_packages(),
    include_package_data=True,
    install_requires=["requests==2.27.1"],
)
