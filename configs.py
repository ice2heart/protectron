from environs import Env
import logging

env = Env()
env.read_env()


logging.basicConfig(level=logging.WARNING)
log = logging.getLogger('protectron')
