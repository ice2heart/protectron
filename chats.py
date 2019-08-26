from aiogram import Bot


async def get_chat_info(bot: Bot, chat_id):
    '''
    TODO: need some caching
    '''
    chat = await bot.get_chat(chat_id)
    # admins = await chat.get_administrators()
    # print(chat['title'], chat['id'], chat, admins)
    # method_to_download local cache and return it
    # images = await bot.get_file(chat['photo']['big_file_id'])
    print(chat)
    return chat
