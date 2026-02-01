import os
from liteboxd import Client


def main() -> None:
    base_url = os.environ.get("LITEBOXD_BASE_URL", "http://localhost:8080/api/v1")
    template = os.environ.get("LITEBOXD_TEMPLATE", "code-interpreter")
    auth_token = os.environ.get("LITEBOXD_AUTH_TOKEN", None)

    client = Client(base_url, auth_token=auth_token)
    sandbox = None
    try:
        sandbox = client.sandbox.create(template=template)
        sandbox = client.sandbox.wait_for_ready(sandbox.id)
        result = client.sandbox.execute(
            sandbox.id,
            command=["python", "-c", "print('hello from liteboxd')"],
        )
        print(result.stdout, end="")
        if result.stderr:
            print(result.stderr, end="")
    finally:
        if sandbox is not None:
            client.sandbox.delete(sandbox.id)
        client.close()


if __name__ == "__main__":
    main()
