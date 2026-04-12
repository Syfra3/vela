class Server:
    def __init__(self, host: str, port: int):
        self.host = host
        self.port = port

    def start(self):
        self.listen()

    def listen(self):
        print(f"Listening on {self.host}:{self.port}")


def create_server(host: str, port: int) -> Server:
    return Server(host, port)
