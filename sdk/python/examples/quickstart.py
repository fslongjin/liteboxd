#!/usr/bin/env python3
"""Quick start example for LiteBoxd Python SDK."""

from liteboxd import Client, SandboxOverrides


def main() -> None:
    # Create a client
    client = Client("http://localhost:8080/api/v1")

    try:
        # Create a sandbox from a template
        print("Creating sandbox...")
        sandbox = client.sandbox.create(
            template="python-data-science",
            overrides=SandboxOverrides(
                ttl=3600,
                env={"DEBUG": "true"},
            ),
        )
        print(f"Created sandbox: {sandbox.id}")

        # Wait for sandbox to be ready
        print("Waiting for sandbox to be ready...")
        sandbox = client.sandbox.wait_for_ready(sandbox.id)
        print(f"Sandbox status: {sandbox.status}")

        # Execute a command
        print("Executing command...")
        result = client.sandbox.execute(
            sandbox.id,
            command=["python", "-c", "print('Hello from LiteBoxd sandbox!')"],
        )
        print(f"Exit code: {result.exit_code}")
        print(f"Output: {result.stdout}")

        # Upload a file
        print("Uploading file...")
        code = b"""
import sys
print(f"Python version: {sys.version}")
print("Hello from uploaded script!")
"""
        client.sandbox.upload_file(
            sandbox.id,
            path="/workspace/script.py",
            content=code,
        )

        # Execute the uploaded script
        print("Running uploaded script...")
        result = client.sandbox.execute(
            sandbox.id,
            command=["python", "/workspace/script.py"],
        )
        print(f"Output: {result.stdout}")

        # Get logs
        print("Getting logs...")
        logs = client.sandbox.get_logs(sandbox.id)
        print(f"Container logs: {logs.logs[:200]}...")

        # Clean up
        print("Deleting sandbox...")
        client.sandbox.delete(sandbox.id)
        print("Done!")

    finally:
        client.close()


if __name__ == "__main__":
    main()
