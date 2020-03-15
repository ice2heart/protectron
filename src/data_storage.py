from datetime import datetime, timezone
from enum import IntEnum


from typing import List, Dict, Tuple
from tinydb import TinyDB, Query

from configs import env, log

db = TinyDB(env.str('DB_PATH', 'db.json'))


class CAPTCHA_STATE(IntEnum):
    INPUTING = 0
    FAIL = 1
    SUCSSES = 2


# from tinydb_serialization import Serializer

# class DateTimeSerializer(Serializer):


class PassStorage:
    def __init__(self, items: List[str], user_id: int, chat_id: int,
                 message_id: int, expired_time: int,
                 debug_id: str,
                 messages: Dict[int, int] = None, input_num: List[str] = None):
        self.items = items
        self.input_num = input_num or []
        self.user_id = user_id
        self.chat_id = chat_id
        self.message_id = message_id
        self.debug_id = debug_id
        if isinstance(expired_time, datetime):
            self.expired_time = expired_time.replace(
                tzinfo=timezone.utc).timestamp()
        else:
            self.expired_time = expired_time
        self.messages = messages or {}

    def add_message_id(self, message_type, message_id):
        self.messages[message_type] = message_id

    def new_char(self, ch):
        self.input_num.append(ch)
        return ''.join(self.input_num)

    def backspace(self):
        try:
            self.input_num.pop()
        except IndexError:
            return
        return ''.join(self.input_num)

    def check(self) -> CAPTCHA_STATE:
        if len(self.input_num) < len(self.items):
            return CAPTCHA_STATE.INPUTING
        log.info(f'{self.input_num} {self.items}')
        if self.input_num == self.items:
            return CAPTCHA_STATE.SUCSSES
        return CAPTCHA_STATE.FAIL

    def user_check(self, user):
        if user != self.user_id:
            return False
        return True

    def is_expired(self, now: datetime) -> bool:
        now_int = now.replace(tzinfo=timezone.utc).timestamp()
        return (self.expired_time < now_int)


class CaptchaStore:
    # TODO: use only db....
    # Sync state.... problem
    def __init__(self):
        self.captcha_store = db.table('captcha', cache_size=30)
        self.current_captcha = {}
        self.load_captcha()

    def load_captcha(self):
        for item in self.captcha_store.all():
            captcha_id = item.pop('item_id')
            log.error(item)
            try:
                self.current_captcha[captcha_id] = PassStorage(**item)
            except TypeError as e:
                log.error(f'Load problem {e!r}')

    def get_captcha(self, captcha_id: str) -> PassStorage:
        return self.current_captcha[captcha_id]

    def new_captcha(self, captcha_id: str, captcha: PassStorage) -> None:
        self.current_captcha[captcha_id] = captcha
        item = vars(captcha)
        item['item_id'] = captcha_id
        self.captcha_store.insert(item)

    def remove_captcha(self, captcha_id: str) -> PassStorage:
        CaptchaItem = Query()
        self.captcha_store.remove(CaptchaItem.item_id == captcha_id)
        return self.current_captcha.pop(captcha_id)

    def list_captcha(self) -> List[Tuple[str, PassStorage]]:
        result = []
        for _id in self.current_captcha:
            result.append((_id, self.current_captcha[_id]))
        return result

    def sync(self):
        pass
