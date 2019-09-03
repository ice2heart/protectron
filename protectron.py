#!/usr/bin/env python3

# import pysnooper
import asyncio
import logging
from enum import Enum
from typing import Dict

import shelve
from datetime import datetime, timedelta
from random import choices, randrange, randint
import json

# import uvloop


# from PIL import Image, ImageDraw

from captcha.image import ImageCaptcha
from environs import Env


from mako.template import Template

from aiogram import Bot, Dispatcher, types
from aiogram.utils import executor
from aiogram.types.message import ContentTypes
from aiogram.types.chat_permissions import ChatPermissions
# from aiogram.utils.exceptions import CantDemoteChatCreator
from aiogram.types import InlineKeyboardMarkup, InlineKeyboardButton, InputFile


# asyncio.set_event_loop_policy(uvloop.EventLoopPolicy())
env = Env()
env.read_env()

API_TOKEN = env.str('API_TOKEN')


# Configure logging
logging.basicConfig(level=logging.INFO)
log = logging.getLogger('protectron')

# Initialize bot and dispatcher
bot: Bot = Bot(token=API_TOKEN)
dp: Dispatcher = Dispatcher(bot)


INPUTING = 0
FAIL = 1
SUCSSES = 2


class MESSAGE_TYPES(Enum):
    LOGIN = 0
    CAPTCHA = 1
    LEFT = 2


class PassStorage:
    def __init__(self, items, user_id, chat_id, message_id, expired_time):
        self.items = items
        self.input_num = []
        self.user_id = user_id
        self.chat_id = chat_id
        self.message_id = message_id
        self.expired_time = expired_time
        self.messages = {}

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

    def check(self):
        if len(self.input_num) < len(self.items):
            return INPUTING
        log.info(f'{self.input_num} {self.items}')
        if self.input_num == self.items:
            return SUCSSES
        return FAIL

    def user_check(self, user):
        if user != self.user_id:
            return False
        return True


data_store = shelve.open('data_store.db', writeback=True)
lang_cache = {}


async def clear(store_item):
    for message in store_item.messages:
        await bot.delete_message(store_item.chat_id, store_item.messages[message])


def s(name: str, params: Dict) -> str:
    if 'lang' not in params:
        log.error(f'Missing required lang on {params}')
        return ''
    lang = params['lang']
    if lang not in lang_cache:
        with open(f'templates/{lang}.json', 'r') as template:
            lang_cache[lang] = json.loads(template.read())
    # TODO: check if it is a list
    template = lang_cache[lang][name]
    return Template(template).render(**params)


@dp.callback_query_handler(lambda c: c.data and c.data.startswith('backspace'))
async def process_callback_backspace(callback_query: types.CallbackQuery):
    debug_id = f'{callback_query.message.chat.username}-({callback_query.from_user.mention})'
    log.info(f'{debug_id}: backspace')
    _id = f'{callback_query.message.message_id}-{callback_query.message.chat.id}'
    try:
        pass_item = data_store[_id]
    except KeyError:
        log.error(f'Something gone wrong: {data_store} {_id}')
        await bot.answer_callback_query(callback_query.id, text=s('something_gone_wrong_warn', {'lang': 'ru'}))
        return
    if not pass_item.user_check(callback_query.from_user.id):
        await bot.answer_callback_query(callback_query.id, text=s('not_for_you_warn', {'lang': 'ru'}))
        return
    text = pass_item.backspace()
    await bot.answer_callback_query(callback_query.id, text=text)


@dp.callback_query_handler(lambda c: c.data and c.data.startswith('btn'))
async def process_callback_kb1btn1(callback_query: types.CallbackQuery):
    code = callback_query.data
    chat_id = callback_query.message.chat.id
    member_id = callback_query.from_user.id
    user_title = callback_query.from_user.mention
    debug_id = f'{callback_query.message.chat.username}-({user_title})'
    chat_title = callback_query.message.chat.title
    log.info(f'{debug_id}: {code}')
    _id = f'{callback_query.message.message_id}-{callback_query.message.chat.id}'
    try:
        pass_item = data_store[_id]
    except KeyError:
        log.error(f'Something gone wrong: {data_store} {_id}')
        await bot.answer_callback_query(callback_query.id, text=s('something_gone_wrong_warn', {'lang': 'ru'}))
        return
    if not pass_item.user_check(callback_query.from_user.id):
        await bot.answer_callback_query(callback_query.id, text=s('not_for_you_warn', {'lang': 'ru'}))
        return
    text = pass_item.new_char(code[-1:])
    result = pass_item.check()
    if result is INPUTING:
        await bot.answer_callback_query(callback_query.id, text=text)
    elif result is SUCSSES:
        data_store.pop(_id)
        log.info(f'{debug_id}: SUCSSES')
        await bot.answer_callback_query(callback_query.id, text='SUCSSES')
        pass_item.messages.pop(MESSAGE_TYPES.LOGIN)
        await clear(pass_item)
        await bot.send_message(callback_query.message.chat.id,
                               s('success_msg', {'lang': 'ru', 'user_title': user_title, 'chat_title': chat_title}))
        unmute = ChatPermissions(can_send_messages=True,
                                 can_send_media_messages=True,
                                 can_add_web_page_previews=True,
                                 can_send_other_messages=True)
        await bot.restrict_chat_member(chat_id, member_id,
                                       permissions=unmute)
    else:
        data_store.pop(_id)
        log.info(f'{debug_id}: FAIL')
        await bot.answer_callback_query(callback_query.id, text=s('fail_msg', {'lang': 'ru'}))
        await clear(pass_item)
        await bot.kick_chat_member(chat_id, member_id)
        await bot.unban_chat_member(chat_id, member_id)

    data_store.sync()


@dp.message_handler(content_types=ContentTypes.LEFT_CHAT_MEMBER)
async def leave_event(message: types.Message):
    for _id in data_store:
        pass_item = data_store[_id]
        if pass_item.chat_id == message.chat.id and pass_item.user_id == message.left_chat_member.id:
            log.info(
                f'{pass_item.chat_id}:@{pass_item.user_id}: Left chat, clean')
            pass_item = data_store.pop(_id)
            pass_item.add_message_id(MESSAGE_TYPES.LEFT, message.message_id)
            await clear(pass_item)
            data_store.sync()
            return


@dp.message_handler(regexp='/ping')
async def ping(message: types.Message):
    log.info(f'Ping requsted {message.chat.title}!')
    await bot.send_message(message.chat.id,
                           text=s('pong_msg', {'lang': 'ru'}),
                           reply_to_message_id=message.message_id)


@dp.message_handler(content_types=ContentTypes.NEW_CHAT_MEMBERS)
async def capcha(message: types.Message):
    mute = ChatPermissions(can_send_messages=False,
                           can_send_media_messages=False,
                           can_add_web_page_previews=False,
                           can_send_other_messages=False)
    my_id = (await bot.me).id
    for member in message.new_chat_members:
        # Do not touch yourself
        if member.id == my_id:
            continue
        if member.is_bot:
            await bot.send_message(message.chat.id,
                                   text=s('join_bot_msg', {'lang': 'ru'}),
                                   reply_to_message_id=message.message_id)
            continue
        # mute user
        user_title = member.mention
        debug_id = f'{message.chat.username}-{user_title}'
        log.info(f'{debug_id}: Start capcha')
        await bot.restrict_chat_member(message.chat.id, member.id, permissions=mute)
        inline_kb_full = InlineKeyboardMarkup(row_width=4)
        # captcha_text_store = 'あかさたなはまやらわがざだばぴぢじぎりみひにちしきぃうぅくすつぬふむゆゅるぐずづぶぷぺべでぜげゑれめねてへせけぇえおこそとのほもよょろをごぞどぼぽ、ゞゝんっゔ'
        captcha_text_store = 'asdfghjkzxcvbnmqwertyu2345678'
        captcha_text = choices(captcha_text_store, k=8)
        btn_text = list(captcha_text)
        btn_pass = list(btn_text)
        btn_order = []
        btns = []
        for _ in range(8):
            random_index = randrange(len(btn_text))
            item = btn_text.pop(random_index)
            _id = f'btn_{item}'
            btn_order.append(btn_pass.index(item))
            btns.append(InlineKeyboardButton(str(item), callback_data=_id))
        inline_kb_full.row(*btns[0:4])
        inline_kb_full.row(*btns[4:8])
        backspace_btn = InlineKeyboardButton('⌫', callback_data='backspace')
        inline_kb_full.row(backspace_btn)

        # file = io.BytesIO()
        # image = Image.new('RGBA', size=(250, 50), color=(155, 0, 0))
        # d = ImageDraw.Draw(image)
        # d.text((10,10), "Hello World", fill=(255,255,0))
        # image.save(file, 'png')

        # image_captcha = ImageCaptcha(fonts=['/usr/share/fonts/truetype/fonts-japanese-gothic.ttf'],  width=460, height=200)
        # image_captcha = ImageCaptcha(fonts=['/home/albert/.local/share/fonts/Iosevka Term Nerd Font Complete.ttf'],  width=460, height=200)
        image_captcha = ImageCaptcha(
            fonts=['NotoSansCJKjp-Regular.otf'],  width=350, height=200)
        image = image_captcha.generate_image(captcha_text)
        for _ in range(randint(1, 5)):
            image_captcha.create_noise_curve(image, image.getcolors())

        # Add noise dots for the image.
        image_captcha.create_noise_dots(image, image.getcolors())
        input_file = InputFile(image_captcha.generate(captcha_text))

        sent_message = await bot.send_photo(message.chat.id, input_file,
                                            caption=s(
                                                'join_msg', {'lang': 'ru', 'user_title': user_title}),
                                            reply_markup=inline_kb_full,
                                            reply_to_message_id=message.message_id)
        _id = f'{sent_message.message_id}-{sent_message.chat.id}'
        expired_time = datetime.now() + timedelta(minutes=5)
        data_store[_id] = PassStorage(
            btn_pass, member.id, sent_message.chat.id, sent_message.message_id, expired_time)
        data_store[_id].add_message_id(MESSAGE_TYPES.LOGIN, message.message_id)
        data_store[_id].add_message_id(
            MESSAGE_TYPES.CAPTCHA, sent_message.message_id)
        data_store.sync()


async def cleaner():
    while True:
        await asyncio.sleep(60)
        if len(data_store) == 0:
            continue
        now = datetime.now()
        for _id in list(data_store.keys()):
            item = data_store[_id]
            if item.expired_time < now:
                log.info(
                    f'{item.chat_id}:@{item.user_id}: Timeout, kick and clean')
                data_store.pop(_id)
                chat_id = item.chat_id
                member_id = item.user_id
                await clear(item)
                await bot.kick_chat_member(chat_id, member_id)
                await bot.unban_chat_member(chat_id, member_id)


if __name__ == '__main__':
    loop = asyncio.get_event_loop()
    asyncio.ensure_future(cleaner())
    executor.start_polling(dp, loop=loop)
    data_store.sync()
    data_store.close()
