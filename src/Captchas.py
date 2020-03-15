from aiogram.types import InlineKeyboardMarkup, InlineKeyboardButton

from random import choices, randrange, randint, choice

import subprocess
from os import path
from tempfile import TemporaryDirectory
from PIL import Image, ImageDraw, ImageFont
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


def gen_pics(text: str, outfile):
    txt = Image.new('RGBA', (224, 224), (45, 52, 54))
    d = ImageDraw.Draw(txt)
    w, h = d.textsize(text, font=fnt)
    x = randint(0, 224-w)
    y = randint(0, 224-h)
    d.text((x, y), text, font=fnt, fill=(223, 230, 233))
    txt.save(outfile)
    # file = io.BytesIO()
    # txt.save(file, 'PNG')
    # return file


def item2btn(item):
    return InlineKeyboardButton(str(item), callback_data=f'btn_{item}')


def video_captcha(lang):
    global fnt
    fnt = ImageFont.truetype(
        '/home/albert/.local/share/fonts/Iosevka Term Nerd Font Complete.ttf', 60)

    x = randint(50, 100)
    y = randint(50, 100)
    symbol = choice(['-', '+'])
    if symbol == '-':
        z = x - y
    elif symbol == '+':
        z = x + y
    else:
        print('Error')
    td = TemporaryDirectory(dir='.')

    gen_pics(f'{x}', path.join(td.name, '1.png'))
    gen_pics(symbol, path.join(td.name, '2.png'))
    gen_pics(f'{y}', path.join(td.name, '3.png'))
    gen_pics('equal?', path.join(td.name, '4.png'))

    line = f'ffmpeg -hide_banner -loglevel panic -y -r 1 -i {path.join(td.name, "%01d.png")} -c:v libx264 -vf fps=1 -pix_fmt yuv420p out.mp4'
    subprocess.call(line.split(' '))
    input_file = InputFile('out.mp4')
    inline_kb_full = InlineKeyboardMarkup(row_width=3)
    inline_kb_full.row(*[item2btn(i) for i in [7, 8, 9]])
    inline_kb_full.row(*[item2btn(i) for i in [4, 5, 6]])
    inline_kb_full.row(*[item2btn(i) for i in [1, 2, 3]])
    inline_kb_full.row(InlineKeyboardButton(
        '⌫', callback_data='backspace'), item2btn(0))
    btn_pass = [c for c in f'{z}']
    print(btn_pass)
    return (input_file, inline_kb_full, btn_pass)


if __name__ == "__main__":
    video_captcha('ru')
