from flask_jwt_extended import get_jwt, verify_jwt_in_request


def jwt_auth_required():
    def wrapper(fn):
        def decorator(*args, **kwargs):
            try:
                verify_jwt_in_request()
            except Exception as e:
                print(e)
                return {"message": "Missing authentication token"}, 401
            claims = get_jwt()
            if not ("file_access_token" in claims and claims["file_access_token"]):
                return fn(*args, **kwargs)
            else:
                return {"message": "Only access token allowed"}, 401

        return decorator

    return wrapper
