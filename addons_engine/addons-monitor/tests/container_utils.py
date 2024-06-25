import random
import uuid


def get_images_list():
    small_images = [
        "busybox",  # ~1.2MB
        "alpine",  # ~5MB
        "node:14-alpine",  # ~39MB
        "python:3.9-alpine",  # ~42MB
    ]

    medium_images = [
        "memcached",  # ~75MB
        "redis",  # ~104MB
        "nginx",  # ~133MB
        "httpd",  # ~166MB
        "rabbitmq",  # ~182MB
    ]

    large_images = [
        "mcr.microsoft.com/dotnet/core/aspnet:3.1",  # ~207MB
        "golang:1.16-alpine",  # ~299MB
        "postgres",  # ~314MB
        "mongo",  # ~493MB
        "mcr.microsoft.com/windows/servercore:ltsc2019",  # ~5GB
    ]

    return [small_images, medium_images, large_images]


def get_dummy_addon(lightweight=True):
    service_name = "test_dummy_service"

    images = get_images_list()
    image_url = random.choice(images[0] if lightweight else random.choice(images))

    return {
        "_id": f"test-{str(uuid.uuid4())}",
        "services": [
            {
                "service_name": service_name,
                "image": image_url,
                "command": "sleep 3600",
                "ports": {},
                "environment": {},
                "networks": [],
            }
        ],
    }


def get_failing_addon():
    service_name = "test_failing_service"

    return {
        "_id": f"test-{str(uuid.uuid4())}",
        "services": [
            {
                "service_name": service_name,
                "image": "busybox",
                "command": "false",
                "ports": {},
                "environment": {},
                "networks": [],
            }
        ],
    }
