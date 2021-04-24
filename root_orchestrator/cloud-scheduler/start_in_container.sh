celery -A cloud_scheduler.celeryapp worker --concurrency=1 --loglevel=DEBUG --uid=nobody --gid=nogroup &

python cloud_scheduler.py &
