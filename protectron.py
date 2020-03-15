#!/usr/bin/env python3

# import pysnooper
import asyncio

from configs import env, log

from enum import IntEnum
from typing import Dict

from datetime import datetime, timedelta
import json

from mako.template import Template

from aiogram import Bot, Dispatcher, types
from aiogram.utils import executor
from aiogram.types.message import ContentTypes
from aiogram.types.chat_permissions import ChatPermissions
# from aiogram.utils.exceptions import CantDemoteChatCreator
from aiogram.utils.exceptions import MessageToDeleteNotFound, NotEnoughRightsToRestrict, MessageCantBeDeleted


from src.data_storage import CAPTCHA_STATE, PassStorage, CaptchaStore
from src.Captchas import base_capthca

# asyncio.set_event_loop_policy(uvloop.EventLoopPolicy())
API_TOKEN = env.str('API_TOKEN')
ADMIN_ID = env.str('ADMIN_ID')

# Initialize bot and dispatcher
bot: Bot = Bot(token=API_TOKEN)
dp: Dispatcher = Dispatcher(bot)


class MESSAGE_TYPES(IntEnum):
    LOGIN = 0
    CAPTCHA = 1
    LEFT = 2


data_store = CaptchaStore()
lang_cache = {}


async def clear(store_item):
    for message in store_item.messages:
        try:
            await bot.delete_message(store_item.chat_id, store_item.messages[message])
        except (MessageToDeleteNotFound, MessageCantBeDeleted):
            pass


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
        pass_item = data_store.get_captcha(_id)
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
        pass_item = data_store.get_captcha(_id)
    except KeyError:
        log.error(f'Something gone wrong: {data_store} {_id}')
        await bot.answer_callback_query(callback_query.id, text=s('something_gone_wrong_warn', {'lang': 'ru'}))
        return
    if not pass_item.user_check(callback_query.from_user.id):
        await bot.answer_callback_query(callback_query.id, text=s('not_for_you_warn', {'lang': 'ru'}))
        return
    text = pass_item.new_char(code[-1:])
    result = pass_item.check()
    if result is CAPTCHA_STATE.INPUTING:
        await bot.answer_callback_query(callback_query.id, text=text)
    elif result is CAPTCHA_STATE.SUCSSES:
        data_store.remove_captcha(_id)
        log.info(f'{debug_id}: SUCSSES')
        await bot.answer_callback_query(callback_query.id, text='SUCSSES')
        try:
            pass_item.messages.pop(MESSAGE_TYPES.LOGIN)
        except KeyError:
            pass
        await clear(pass_item)
        await bot.send_message(callback_query.message.chat.id,
                               s('success_msg', {'lang': 'ru', 'user_title': user_title, 'chat_title': chat_title}))
        unmute = ChatPermissions(can_send_messages=True,
                                 can_send_media_messages=True,
                                 can_add_web_page_previews=True,
                                 can_send_other_messages=True,
                                 can_send_polls=True)
        await bot.restrict_chat_member(chat_id, member_id,
                                       permissions=unmute)
    else:
        data_store.remove_captcha(_id)
        log.info(f'{debug_id}: FAIL')
        await bot.answer_callback_query(callback_query.id, text=s('fail_msg', {'lang': 'ru'}))
        await clear(pass_item)
        await bot.kick_chat_member(chat_id, member_id)
        await bot.unban_chat_member(chat_id, member_id)

    data_store.sync()


@dp.message_handler(content_types=ContentTypes.LEFT_CHAT_MEMBER)
async def leave_event(message: types.Message):
    for _id, pass_item in data_store.list_captcha():
        if pass_item.chat_id == message.chat.id and pass_item.user_id == message.left_chat_member.id:
            log.info(
                f'{pass_item.chat_id}:@{pass_item.user_id}: Left chat, clean')
            pass_item = data_store.remove_captcha(_id)
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
                           can_send_other_messages=False,
                           can_send_polls=False)
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
        if member.id == ADMIN_ID:
            await bot.send_message(message.chat_id, text=s('join_owner_msg', {'lang': 'ru'}),
                                   reply_to_message_id=message.message_id)
        # mute user
        user_title = member.mention
        debug_id = f'{message.chat.username}-{user_title}'
        log.info(f'{debug_id}: Start capcha')
        try:
            await bot.restrict_chat_member(message.chat.id, member.id, permissions=mute)
        except NotEnoughRightsToRestrict as e:
            log.info(f'{debug_id} can\'t restrict member : {e}')
            continue

        input_file, inline_kb_full, btn_pass = base_capthca('ru')
        sent_message = await bot.send_photo(message.chat.id, input_file,
                                            caption=s(
                                                'join_msg', {'lang': 'ru', 'user_title': user_title}),
                                            reply_markup=inline_kb_full,
                                            reply_to_message_id=message.message_id)
        _id = f'{sent_message.message_id}-{sent_message.chat.id}'
        expired_time = datetime.now() + timedelta(minutes=5)
        pass_item = PassStorage(
            btn_pass, member.id, sent_message.chat.id, sent_message.message_id, expired_time, debug_id)
        pass_item.add_message_id(MESSAGE_TYPES.LOGIN, message.message_id)
        pass_item.add_message_id(
            MESSAGE_TYPES.CAPTCHA, sent_message.message_id)
        data_store.new_captcha(_id, pass_item)
        data_store.sync()


async def cleaner():
    while True:
        try:
            await asyncio.sleep(60)
            now = datetime.now()
            for _id, item in data_store.list_captcha():
                if item.is_expired(now):
                    log.info(
                        f'{item.debug_id}: Timeout, kick and clean')
                    data_store.remove_captcha(_id)
                    chat_id = item.chat_id
                    member_id = item.user_id
                    await clear(item)
                    await bot.kick_chat_member(chat_id, member_id)
                    await bot.unban_chat_member(chat_id, member_id)
        except Exception as e:
            log.error(f'Uncaught exception {e}')


if __name__ == '__main__':
    loop = asyncio.get_event_loop()
    asyncio.ensure_future(cleaner())
    executor.start_polling(dp, loop=loop)
