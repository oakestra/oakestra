import logging
from datetime import datetime

import jwt
from ext_requests.jwt_generator_requests import create_access_token, create_refresh_token
from ext_requests.user_db import mongo_get_user_by_name
from flask_jwt_extended import get_jwt, get_jwt_identity, jwt_required, verify_jwt_in_request
from jwt import InvalidTokenError

logger = logging.getLogger("system_manager")


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
                logger.error(e)
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


def cluster_token_expired(token):
    """Check exp claim without verifying signature (token is compared to stored value first)."""
    try:
        payload = jwt.decode(
            token,
            options={"verify_signature": False, "verify_exp": False},
            algorithms=["RS256"],
        )
    except InvalidTokenError as e:
        logger.warning(f"Invalid cluster token format: {str(e)}")
        return True

    exp = payload.get("exp")
    if exp is None:
        logger.warning("Cluster token has no exp claim")
        return True

    try:
        return datetime.utcnow().timestamp() >= float(exp)
    except (TypeError, ValueError):
        logger.warning("Cluster token has invalid exp claim")
        return True


def verify_cluster_token(cluster_name, provided_token):
    """
    Verify that the provided token matches the stored cluster token.
    
    Args:
        cluster_name: The name of the cluster
        provided_token: The token provided by the cluster
    
    Returns:
        True if token is valid and matches stored token, False otherwise
    """
    from ext_requests.token_db import get_cluster_token, mark_cluster_token_used
    
    try:
        token_doc = get_cluster_token(cluster_name)
        if not token_doc:
            logger.warning(f"No cluster token found for {cluster_name}")
            return False

        if token_doc.get("used", False):
            logger.warning(f"Cluster token for {cluster_name} was already used")
            return False

        stored_token = token_doc.get("token", "")
        if not stored_token or provided_token != stored_token:
            logger.warning(f"Invalid cluster token provided for {cluster_name}")
            return False

        if cluster_token_expired(stored_token):
            logger.warning(f"Expired cluster token provided for {cluster_name}")
            return False

        if not mark_cluster_token_used(cluster_name, stored_token):
            logger.warning(
                f"Cluster token for {cluster_name} could not be marked as used"
            )
            return False

        logger.info(f"Cluster token verified for {cluster_name}")
        return True
    except Exception as e:
        logger.error(f"Error verifying cluster token for {cluster_name}: {str(e)}")
        return False
