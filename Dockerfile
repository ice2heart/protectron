FROM python:3.11-alpine

ADD . /protectron
WORKDIR /protectron
RUN apk add --no-cache --virtual .build-deps jpeg-dev zlib-dev freetype-dev  build-base linux-headers \
    && pip install pipenv \
    && cd /protectron; pipenv install \
    && apk del .build-deps
CMD [ "pipenv", "run", "python", "protectron.py" ]