from functools import wraps

from flask_jwt_extended import jwt_required
from flask_smorest import Blueprint


class BlueprintExt(Blueprint):
    @staticmethod
    def doc_jwt_required():
        @wraps
        def wrapper(func):
            @jwt_required
            def inner(*args, **kwargs):
                parameters = [
                    {
                        "name": "Authorization",
                        "in": "header",
                        "description": "Authorization: Bearer <access_token>",
                        "required": "true",
                    }
                ]

                func._apidoc = getattr(func, "_apidoc", {})
                func._apidoc.setdefault("parameters", []).append(parameters)
                func(*args, **kwargs)

            return inner

        return wrapper
