from aiogram.filters.callback_data import CallbackData

class KeyboardCallback(CallbackData, prefix='key'):
    key: str
    value: str

