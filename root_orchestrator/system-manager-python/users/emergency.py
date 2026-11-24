"""
Emergency Admin login for when the identity provider is down or misconfigured.

Run inside the system manager container:

    docker exec system_manager python -m users.emergency --minutes 60

It prints one JSON line {"code": ..., "expires_in": 600, "session_minutes": N}. Exchanging the
code at POST /api/auth/oidc/exchange (or opening <dashboard>/#oidc_code=<code>) gives an Admin
session for the local user "emergency-admin" with the claim emergency=true. The session cannot be
renewed past N minutes. Only someone who can run commands in the container can do this; every use
is logged as a warning.
"""

import argparse
import json
import logging
import sys
from datetime import datetime

import ext_requests.mongodb_client as db
from ext_requests import organization_db, user_db
from roles.securityUtils import Role

from users import sessions

logger = logging.getLogger("system_manager")

EMERGENCY_USER = "emergency-admin"
EMERGENCY_ROLES = [Role.ADMIN, Role.ORGANIZATION_ADMIN, Role.APP_Provider, Role.INF_Provider]
MAX_MINUTES = 24 * 60


def ensure_emergency_user():
    """The local emergency-admin user with Admin roles in the root organization."""
    user_db.create_admin()  # makes sure the root organization exists
    user = user_db.mongo_get_user_by_name(EMERGENCY_USER)
    if user is None:
        user_id = str(
            user_db.mongo_save_user_without_roles(
                {
                    "name": EMERGENCY_USER,
                    "email": "",
                    "password": None,
                    "emergency": True,
                    "created_at": datetime.now().strftime("%d/%m/%Y %H:%M"),
                }
            )
        )
    elif user.get("oidc_iss") or user.get("federated"):
        raise RuntimeError(f"{EMERGENCY_USER} exists as a federated user; refusing to use it")
    else:
        user_id = str(user["_id"])
    organization_db.mongo_add_user_role_to_organization(
        user_id, sessions.root_organization_id(), EMERGENCY_ROLES
    )


def create_emergency_code(minutes):
    if minutes < 1 or minutes > MAX_MINUTES:
        raise ValueError(f"--minutes must be between 1 and {MAX_MINUTES}")
    ensure_emergency_user()
    code = sessions.create_code(
        {
            "username": EMERGENCY_USER,
            "roles": EMERGENCY_ROLES,
            "emergency": True,
            "session_minutes": minutes,
        },
        ttl=sessions.EMERGENCY_CODE_TTL,
    )
    logger.warning("EMERGENCY login code created for a %d minute Admin session", minutes)
    return {
        "code": code,
        "expires_in": int(sessions.EMERGENCY_CODE_TTL.total_seconds()),
        "session_minutes": minutes,
    }


def main(argv=None):
    parser = argparse.ArgumentParser(description="Create a one-time emergency Admin login code")
    parser.add_argument("--minutes", type=int, default=60, help="session length (default 60)")
    args = parser.parse_args(argv)
    logging.basicConfig(level=logging.WARNING, stream=sys.stderr)
    db.mongo_init_standalone()
    try:
        result = create_emergency_code(args.minutes)
    except (ValueError, RuntimeError) as e:
        print(json.dumps({"error": str(e)}))
        return 1
    print(json.dumps(result))
    return 0


if __name__ == "__main__":
    sys.exit(main())
