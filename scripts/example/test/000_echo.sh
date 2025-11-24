#!/bin/bash

echo "USER=$USER"
echo "DEBUG=$DEBUG"
echo "RETRIES=$RETRIES"
echo "STATIC_ENV=$STATIC_ENV"

if [ "$DEBUG" = "true" ]; then
  echo "Debug mode enabled!"
fi
