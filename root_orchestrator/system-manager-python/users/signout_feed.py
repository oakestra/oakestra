"""
Optional sign-out feed (OIDC_SIGNOUT_FEED_URL): every 60 s the system manager asks the provider
which users lost access since the last poll and signs them out right away instead of at their next
renewal.

    GET <feed>?since=<"now" of the previous answer>   (HTTP Basic client_id:client_secret)
    -> {"signouts": [{"sub": str, "at": ISO8601}], "now": ISO8601}
"""

import logging
import threading
from datetime import datetime, timezone

import ext_requests.mongodb_client as db
import requests

from users import sessions
from users.federated import settings

logger = logging.getLogger("system_manager")

POLL_SECONDS = 60

_state = {"since": None}


def parse_time(value):
    try:
        parsed = datetime.fromisoformat(str(value).replace("Z", "+00:00"))
    except ValueError:
        return None
    return parsed if parsed.tzinfo else parsed.replace(tzinfo=timezone.utc)


def poll_once():
    """One poll; returns the number of users signed out."""
    s = settings()
    params = {"since": _state["since"]} if _state["since"] else {}
    r = requests.get(
        s.signout_feed_url,
        params=params,
        auth=(s.client_id, s.client_secret),
        timeout=10,
    )
    r.raise_for_status()
    body = r.json()
    # the ID token's iss may differ from OIDC_ISSUER by a trailing slash
    issuers = [s.issuer.rstrip("/"), s.issuer.rstrip("/") + "/"]
    count = 0
    for event in body.get("signouts") or []:
        sub = event.get("sub") if isinstance(event, dict) else None
        if not isinstance(sub, str):
            continue
        user = db.mongo_users.find_one({"oidc_iss": {"$in": issuers}, "oidc_sub": sub})
        if user is None:
            continue
        sessions.signout_user(user["name"], parse_time(event.get("at")) or sessions.now())
        count += 1
    if body.get("now"):
        _state["since"] = body["now"]
    return count


def _loop(stop):
    while not stop.wait(POLL_SECONDS):
        try:
            poll_once()
        except Exception as e:
            logger.warning("Sign-out feed poll failed: %s", e)


def start():
    """Starts the poller thread if OIDC_SIGNOUT_FEED_URL is set; returns the stop event or None."""
    s = settings()
    if not s.signout_feed_url or not s.issuer:
        return None
    stop = threading.Event()
    threading.Thread(target=_loop, args=(stop,), daemon=True, name="oidc-signout-feed").start()
    logger.info("Polling the sign-out feed %s every %d s", s.signout_feed_url, POLL_SECONDS)
    return stop
