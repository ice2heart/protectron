import hmac
import hashlib
from typing import Dict


def auth_check(user: Dict, secret: str):
    '''
    Check auth
    '''
    encoded_secret = hashlib.sha256()
    encoded_secret.update(secret.encode())
    _hash = user.pop('hash')
    items = list(user.keys())
    items.sort()
    items = [f'{x}={str(user[x])}' for x in items]
    line = '\n'.join(items)
    localhash = hmac.new(encoded_secret.digest(), line.encode(
        'utf-8'), hashlib.sha256).hexdigest()
    return hmac.compare_digest(_hash, localhash)
