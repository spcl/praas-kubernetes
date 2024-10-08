#!/bin/sh

# Check if redis is running
REDIS_IS_RUNNING=$(redis-cli ping 2>&1)

# If it's not then start redis
if [ "$REDIS_IS_RUNNING" != "PONG" ]; then
    redis-server &
fi

# Import python virtualenv
. venv/bin/activate

# Run the server
export FLASK_ENV=development
export FLASK_APP=server
flask run --host=127.0.1.2