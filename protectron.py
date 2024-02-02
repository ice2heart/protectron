#!/usr/bin/env python3

# import pysnooper
import asyncio

from configs import env, log

from enum import IntEnum
from typing import Dict

from datetime import datetime, timedelta
import json

from mako.template import Template

from aiogram import Bot, Dispatcher, types, Router, F
from aiogram.enums import ParseMode
from aiogram.types import ContentType, ChatMemberUpdated
from aiogram.filters import Command, CommandObject, IS_MEMBER, IS_NOT_MEMBER, ChatMemberUpdatedFilter
from aiogram.types.chat_permissions import ChatPermissions
from aiogram.types.error_event import ErrorEvent
from aiogram.exceptions import AiogramError, TelegramBadRequest

from src.data_storage import CAPTCHA_STATE, PassStorage, CaptchaStore
from src.Captchas import base_capthca
from src.Callback import KeyboardCallback

# asyncio.set_event_loop_policy(uvloop.EventLoopPolicy())
API_TOKEN = env.str('API_TOKEN')
ADMIN_ID = int(env.str('ADMIN_ID'))

# Initialize bot and dispatcher
bot: Bot = Bot(token=API_TOKEN, parse_mode=ParseMode.HTML)
dp: Dispatcher = Dispatcher()
router: Router = Router()
dp.include_router(router)


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
        except AiogramError:
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



@router.callback_query(KeyboardCallback.filter(F.key == 'backspace'))
async def process_callback_backspace(callback_query: types.CallbackQuery, callback_data: KeyboardCallback):
    debug_id = f'{callback_query.message.chat.username}-({callback_query.from_user.full_name})'
    log.warning(f'{debug_id}: backspace')
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


@router.callback_query(KeyboardCallback.filter(F.key == 'btn'))
async def process_callback_kb1btn1(callback_query: types.CallbackQuery, callback_data: KeyboardCallback):
    code = callback_data.value
    chat_id = callback_query.message.chat.id
    member_id = callback_query.from_user.id
    user_title = callback_query.from_user.full_name
    debug_id = f'{callback_query.message.chat.username}-({user_title})'
    chat_title = callback_query.message.chat.title
    log.warning(f'{debug_id}: {code}')
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
    text = pass_item.new_char(code)
    result = pass_item.check()
    if result is CAPTCHA_STATE.INPUT:
        await bot.answer_callback_query(callback_query.id, text=text)
    elif result is CAPTCHA_STATE.SUCCESS:
        data_store.remove_captcha(_id)
        log.warning(f'{debug_id}: SUCCESS')
        await bot.answer_callback_query(callback_query.id, text='SUCCESS')
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
        delay = datetime.now() + timedelta(minutes=1)
        log.warning(f'{debug_id}: FAIL, ban until {delay}')
        # workaround to send new event.
        await bot.ban_chat_member(chat_id, member_id, delay)
        # await bot.kick_chat_member(chat_id, member_id, delay)
        await bot.answer_callback_query(callback_query.id, text=s('fail_msg', {'lang': 'ru'}))
        await clear(pass_item)
        await bot.unban_chat_member(chat_id, member_id)

    data_store.sync()

@router.error()
async def error_handler(event: ErrorEvent):
    log.critical("Critical error caused by %s", event.exception, exc_warning=True)

@router.chat_member(ChatMemberUpdatedFilter(IS_MEMBER >> IS_NOT_MEMBER))
async def leave_event(event: ChatMemberUpdated):
    for _id, pass_item in data_store.list_captcha():
        if pass_item.chat_id == event.chat.id and pass_item.user_id == event.from_user.id:
            log.warning(
                f'{pass_item.chat_id}:@{pass_item.user_id}: Left chat, clean')
            pass_item = data_store.remove_captcha(_id)
            # pass_item.add_message_id(MESSAGE_TYPES.LEFT, message.message_id)
            await clear(pass_item)
            data_store.sync()
            return


@router.message(Command("ping"))
async def ping(message: types.Message, command: CommandObject):
    log.warning(f'Ping requsted {message.chat.title}!')
    try:
        await bot.send_message(message.chat.id,
                            text=s('pong_msg', {'lang': 'ru'}),
                            reply_to_message_id=message.message_id)
    except TelegramBadRequest:
        pass


@router.chat_member(ChatMemberUpdatedFilter(IS_NOT_MEMBER >> IS_MEMBER))
async def capcha(event: ChatMemberUpdated):
    # event.new_chat_member
    mute = ChatPermissions(can_send_messages=False,
                           can_send_media_messages=False,
                           can_add_web_page_previews=False,
                           can_send_other_messages=False,
                           can_send_polls=False)
    my_id = (await bot.me()).id
    member = event.new_chat_member.user
    # member.user.id
    # Do not touch yourself
    if member.id == my_id:
        return
    user_title = member.full_name
    debug_id = f'{event.chat.username}-{user_title}:{member.id}'
    if member.is_bot:
        await event.answer(text=s('join_bot_msg', {'lang': 'ru'}))
        return
    if member.id == ADMIN_ID:
        await event.answer(text=s('join_owner_msg', {'lang': 'ru'}))
        return
    # mute user

    log.warning(f'{debug_id}: Start capcha')
    try:
        await bot.restrict_chat_member(event.chat.id, member.id, permissions=mute)
    except AiogramError as e:
        log.warning(f'{debug_id} can\'t restrict member : {e}')
        try:
            await event.answer( text=s('required_admin_permission', {'lang': 'en'}))
        except AiogramError as e:
            log.warning(f'{debug_id} Exception {e!r}')
        await bot.leave_chat(event.chat.id)
        return

    input_file, inline_kb_full, btn_pass = base_capthca('ru')
    log.warning(f'captcha: {btn_pass}')
    sent_message = await event.answer_photo( input_file,
                                        caption=s(
                                            'join_msg', {'lang': 'ru', 'user_title': user_title}),
                                        reply_markup=inline_kb_full)
    _id = f'{sent_message.message_id}-{sent_message.chat.id}'
    expired_time = datetime.now() + timedelta(minutes=5)
    pass_item = PassStorage(
        btn_pass, member.id, sent_message.chat.id, sent_message.message_id, expired_time, debug_id)
    # pass_item.add_message_id(MESSAGE_TYPES.LOGIN, event.message_id)
    pass_item.add_message_id(
        MESSAGE_TYPES.CAPTCHA, sent_message.message_id)
    data_store.new_captcha(_id, pass_item)
    data_store.sync()


async def cleaner():
    while True:
        await asyncio.sleep(60)
        now = datetime.now()
        for _id, item in data_store.list_captcha():
            try:
                if not item.is_expired(now):
                    continue
                log.warning(
                    f'{item.debug_id}: Timeout, kick and clean')
                chat_id = item.chat_id
                member_id = item.user_id
                await clear(item)
                delay = datetime.now() + timedelta(minutes=1)
                await bot.ban_chat_member(chat_id, member_id,delay)
                data_store.remove_captcha(_id)
                await bot.unban_chat_member(chat_id, member_id)
            except AiogramError as e:
                log.error(f'Unauthorized: {e}')
                data_store.remove_captcha(_id)
            except Exception as e:
                log.error(f'Uncaught exception {e}')

async def main():
    # And the run events dispatching
    pulling =  dp.start_polling(bot)
    cleaner_future = cleaner()
    # fix ctrl+c
    await asyncio.gather(pulling, cleaner_future)

if __name__ == '__main__':
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    # asyncio.ensure_future(cleaner())
    asyncio.run(main())