interface Logger {
  log(message: string): void;
}

class Server {
  private host: string;
  private port: number;

  constructor(host: string, port: number) {
    this.host = host;
    this.port = port;
  }

  start(): void {
    this.listen();
  }

  private listen(): void {
    console.log(`Listening on ${this.host}:${this.port}`);
  }
}

function createServer(host: string, port: number): Server {
  return new Server(host, port);
}

const handleRequest = (req: string): void => {
  console.log(req);
};
