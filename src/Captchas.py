from aiogram.types import InlineKeyboardMarkup, InlineKeyboardButton

from random import choices, randrange, randint

# import uvloop
# from PIL import Image, ImageDraw
from captcha.image import ImageCaptcha
from aiogram.types import InputFile


def base_capthca(lang):
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

    return (input_file, inline_kb_full, btn_pass)
