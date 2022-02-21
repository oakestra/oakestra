import os
from app import create_app
from app.blueprints.node_engine.node_engine import init_node_engine
from app import celery
if __name__ == "__main__":
    app = create_app()
    init_node_engine()
    my_port = os.environ.get('PUBLIC_WORKER_PORT') or 3000
    app.run(host='0.0.0.0', port=my_port)
