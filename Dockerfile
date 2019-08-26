FROM python:3.7-alpine

RUN apk add --no-cache jpeg-dev zlib-dev freetype-dev 
ADD . /protectron
WORKDIR /protectron
RUN apk add --no-cache --virtual .build-deps build-base linux-headers \
    && pip install pipenv \
    && cd /protectron; pipenv install \
    && apk del .build-deps
CMD [ "pipenv", "run", "python", "protectron.py" ]