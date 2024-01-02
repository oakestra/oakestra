from ext_requests.user_db import mongo_get_user_by_name
from flask_jwt_extended import (
    create_access_token,
    create_refresh_token,
    get_jwt,
    get_jwt_identity,
    jwt_required,
    verify_jwt_in_request,
)


class Role:
    ADMIN = "Admin"
    ORGANIZATION_ADMIN = "Organization_Admin"
    APP_Provider = "Application_Provider"
    INF_Provider = "Infrastructure_Provider"


not_authorized = {"message": "You have not enough permissions!"}, 403


def user_has_role(user, role):
    claims = get_jwt_auth_claims()
    return role in claims["roles"]


def require_role(required_role):
    def decorator(func):
        def wrapper(*args, **kwargs):
            current_user = get_jwt_auth_identity()
            organization_id = get_jwt_organization()
            user = mongo_get_user_by_name(current_user, organization_id)
            if user and user_has_role(user, required_role):
                return func(*args, **kwargs)
            else:
                return not_authorized

        return wrapper

    return decorator


def identity_is_username():
    def decorator(func):
        def wrapper(*args, **kwargs):
            current_user = get_jwt_auth_identity()
            if current_user == kwargs["username"]:
                return func(*args, **kwargs)
            else:
                return not_authorized

        return wrapper

    return decorator


def has_access_to_user(username):
    user = get_jwt_auth_identity()
    return username == user


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


def get_jwt_auth_identity():
    return get_jwt_identity()


def get_jwt_auth_claims():
    return get_jwt()


def get_jwt_organization():
    return get_jwt_auth_claims()["organization"]


def refresh_token_required():
    return jwt_required(refresh=True)


def create_jwt_auth_access_token(identity, additional_claims):
    return create_access_token(identity=identity, additional_claims=additional_claims)


def create_jwt_auth_refresh_token(identity, additional_claims):
    return create_refresh_token(identity=identity, additional_claims=additional_claims)
