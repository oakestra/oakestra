import hashlib
import logging
import os
from datetime import datetime

import ext_requests.user_db
from ext_requests import organization_db, user_db
from flask import abort
from mail import mail
from mail.mail import ResetPasswordMailFactory
from roles import securityUtils
from werkzeug.security import check_password_hash, generate_password_hash

mail_user = os.environ.get("MAIL_USER", "")


def user_register(content, organization_id):
    if len(content["name"]) > 0 and len(content["password"]) > 0:
        existing_user = user_db.mongo_get_user_by_name(content["name"])
        if existing_user is not None:
            return {"message": "Username already exists"}, 409

        password = content["password"]
        content["password"] = generate_password_hash(content["password"])
        if "_id" in content:
            del content["_id"]
        user = user_db.mongo_save_user(content, organization_id)
        content["password"] = password

        if mail_user != "":
            (mail.RegistrationMailFactory(content)).send_mail()

        return user
    else:
        return {"message": "Invalid information"}, 404


def user_login(content):
    """
    Log in a user to the platform
    requires content={
                        username:string
                        password:string
                        organization_name: string (optional)
                    }
    """

    if content is None:
        return {"message": "no credentials provided"}
    username = content["username"]
    password = content["password"]
    organization_struct = None

    if "organization_name" in content:
        organization_struct = organization_db.mongo_get_organization_by_name(
            content["organization_name"]
        )
        if organization_struct is None:
            abort(404, {"message": "Found no organization with this name"})
    else:
        organization_struct = organization_db.mongo_get_organization_by_name("root")

    organization_id = str(organization_struct["_id"])

    if len(username) > 0 and len(password) > 0:
        user_struct = user_db.mongo_get_user_by_name(username)

        logging.log(logging.INFO, user_struct)
        if user_struct is not None:
            logging.log(level=logging.ERROR, msg="User not found")
            roles = get_user_roles_from_organization(organization_struct, str(user_struct["_id"]))
            if check_password_hash(user_struct.get("password"), password):
                access_token = securityUtils.create_jwt_auth_access_token(
                    identity=username,
                    additional_claims={
                        "user": username,
                        "roles": roles,
                        "organization": organization_id,
                    },
                )
                refresh_token = securityUtils.create_jwt_auth_refresh_token(
                    identity=username,
                    additional_claims={
                        "user": username,
                        "roles": roles,
                        "organization": organization_id,
                    },
                )

                return {"token": access_token, "refresh_token": refresh_token}

            else:
                logging.log(
                    level=logging.ERROR,
                    msg="Invalid password provided from user: " + username,
                )
        else:
            logging.log(level=logging.ERROR, msg="User not found: " + username)
    else:
        logging.log(level=logging.ERROR, msg="Invalid credentials")

    return {}


def user_token_refresh(username):
    user = ext_requests.user_db.mongo_get_user_by_name(username)
    claims = securityUtils.get_jwt_auth_claims()

    if user is None:
        return {}
    return {
        "token": securityUtils.create_jwt_auth_access_token(
            identity=username,
            additional_claims={
                "user": username,
                "roles": claims["roles"],
                "organization": claims["organization"],
            },
        )
    }


def user_get_roles(username, organization_id):
    return user_db.mongo_get_user_by_name(username, organization_id)


def user_change_password(username, oldpw, newpw):
    """
    Admin changes user password with a new one, return the status code of the operation
    """
    user = user_db.mongo_get_user_by_name(username)
    if user is None:
        return {"message": "User does not exists"}, 404

    current_password = user["password"]

    if not check_password_hash(current_password, oldpw):
        return {"message": "Old password is not valid!"}, 400

    if check_password_hash(current_password, newpw):
        return {"message": "Old password can't be the new password!"}, 400

    user["password"] = generate_password_hash(newpw)

    user_db.mongo_update_user(user["_id"], user)

    return {}, 200


def user_create_password_reset_request(username, domain, reset_token, expiration):
    user = user_db.mongo_get_user_by_name(username)
    if user is None:
        return {"message": "User does not exists"}, 404

    reset_token_hash = hashlib.pbkdf2_hmac("sha256", reset_token.encode("ascii"), b"", 100000).hex()
    user_db.mongo_create_password_reset_token(
        user_id=user["_id"], expiry_date=expiration, token_hash=reset_token_hash
    )

    email = {
        "link": "http://" + domain + "/resetPassword/" + reset_token_hash,
        "expiry_delta": expiration,
    }

    (ResetPasswordMailFactory(user, email)).send_mail()

    return {}, 200


def user_change_password_with_reset_request(reset_token, new_password):
    new_password = generate_password_hash(new_password)
    reset_token_hash = hashlib.pbkdf2_hmac("sha256", reset_token.encode("ascii"), b"", 100000).hex()
    try:
        token = user_db.mongo_get_password_reset_token(reset_token_hash)
    except Exception:
        return {"message": "Link invalid! Please request a new one!"}, 400
    if token is not None:
        user_db.mongo_delete_password_reset_token(token["_id"])

    if token is None or datetime.now() >= token["expiry_date"]:
        return {"message": "Link expired! Please request a new one!"}, 400
    else:
        user = user_db.mongo_get_user_by_id(token["user_id"])
        user["password"] = new_password
        user_db.mongo_update_user(user["_id"], user)
    return {}, 200


def get_user_roles_from_organization(organization, user_id):
    for member in organization["member"]:
        if member["user_id"] == user_id:
            print(member["roles"])
            return member["roles"]
    return []
