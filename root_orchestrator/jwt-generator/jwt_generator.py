import os
from datetime import timedelta

from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from flask import Flask, jsonify, request
from flask_jwt_extended import JWTManager, create_access_token, create_refresh_token

private_key = os.getenv("JWT_PRIVATE_KEY")
public_key = os.getenv("JWT_PUBLIC_KEY")

if not private_key or not public_key:
    private_key_obj = rsa.generate_private_key(
        public_exponent=65537,
        key_size=2048,
    )
    public_key_obj = private_key_obj.public_key()

    private_key = private_key_obj.private_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PrivateFormat.TraditionalOpenSSL,
        encryption_algorithm=serialization.NoEncryption(),
    ).decode("utf-8")
    public_key = public_key_obj.public_bytes(
        encoding=serialization.Encoding.PEM, format=serialization.PublicFormat.SubjectPublicKeyInfo
    ).decode("utf-8")

app = Flask(__name__)
app.config["JWT_PRIVATE_KEY"] = private_key
app.config["JWT_PUBLIC_KEY"] = public_key
app.config["JWT_ALGORITHM"] = "RS256"
app.config["JWT_ACCESS_TOKEN_EXPIRES"] = timedelta(minutes=10)
app.config["RESET_TOKEN_EXPIRES"] = timedelta(hours=3)

jwt = JWTManager(app)


@app.route("/create", methods=["POST"])
def create_access_token_route():
    if not request.is_json:
        return jsonify({"msg": "Missing JSON in request"}), 400

    identity = request.json.get("identity", None)
    fresh = parse_timedelta(request.json.get("fresh", None), False)
    expires_delta = parse_timedelta(request.json.get("expires_delta", None), None)
    additional_claims = request.json.get("additional_claims", None)
    additional_headers = request.json.get("additional_headers", None)

    access_token = create_access_token(
        identity=identity,
        fresh=fresh,
        expires_delta=expires_delta,
        additional_claims=additional_claims,
        additional_headers=additional_headers,
    )

    return jsonify(access_token=access_token), 200


@app.route("/refresh", methods=["POST"])
def create_refresh_token_route():
    if not request.is_json:
        return jsonify({"msg": "Missing JSON in request"}), 400

    identity = request.json.get("identity", None)
    expires_delta = parse_timedelta(request.json.get("expires_delta", None), None)
    additional_claims = request.json.get("additional_claims", None)
    additional_headers = request.json.get("additional_headers", None)

    if expires_delta is not None:
        expires_delta = timedelta(minutes=expires_delta)

    refresh_token = create_refresh_token(
        identity=identity,
        expires_delta=expires_delta,
        additional_claims=additional_claims,
        additional_headers=additional_headers,
    )

    return jsonify(refresh_token=refresh_token), 200


@app.route("/key", methods=["GET"])
def get_public_key_route():
    return jsonify(public_key=public_key), 200


@app.route("/health", methods=["GET"])
def get_health():
    if not private_key or not public_key:
        return "Keys not available!", 500
    return "Working!", 200


def parse_timedelta(encoded_object, defaultValue):
    try:
        return timedelta(
            days=encoded_object.get("days"),
            seconds=encoded_object.get("seconds"),
            microseconds=encoded_object.get("microseconds"),
        )
    except Exception:
        return defaultValue


if __name__ == "__main__":
    host = os.getenv("JWT_GENERATOR_HOST", "0.0.0.0")
    port = int(os.getenv("JWT_GENERATOR_PORT", 5000))
    app.run(debug=True, port=port, host=host)
