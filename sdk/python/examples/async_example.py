#!/usr/bin/env python3
"""Async example for LiteBoxd Python SDK."""

import asyncio

from liteboxd import AsyncClient, SandboxOverrides


async def main() -> None:
    async with AsyncClient("http://localhost:8080/api/v1") as client:
        # Create a sandbox from a template
        print("Creating sandbox...")
        sandbox = await client.sandbox.create(
            template="python-data-science",
            overrides=SandboxOverrides(ttl=3600),
        )
        print(f"Created sandbox: {sandbox.id}")

        # Wait for sandbox to be ready
        print("Waiting for sandbox to be ready...")
        sandbox = await client.sandbox.wait_for_ready(sandbox.id)
        print(f"Sandbox status: {sandbox.status}")

        # Execute multiple commands concurrently
        print("Executing commands concurrently...")
        results = await asyncio.gather(
            client.sandbox.execute(sandbox.id, ["python", "-c", "print('Task 1')"]),
            client.sandbox.execute(sandbox.id, ["python", "-c", "print('Task 2')"]),
            client.sandbox.execute(sandbox.id, ["python", "-c", "print('Task 3')"]),
        )

        for i, result in enumerate(results, 1):
            print(f"Task {i} output: {result.stdout.strip()}")

        # Clean up
        print("Deleting sandbox...")
        await client.sandbox.delete(sandbox.id)
        print("Done!")


if __name__ == "__main__":
    asyncio.run(main())
