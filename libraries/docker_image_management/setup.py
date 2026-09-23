from setuptools import find_packages, setup

setup(
    name="docker_image_management",
    version="0.1.0",
    description="Shared docker image management utilities for Oakestra services.",
    packages=find_packages(),
    include_package_data=True,
    install_requires=["docker>=7.0.0"],
)
