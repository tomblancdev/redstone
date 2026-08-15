"""The Python binder for redstone - deliberately micro: bind, optional,
edge identity. Generated stubs ship inside the package (_gen). If this file
grows features, it is failing.

    from redstone_sdk import Client

    c = Client(register="register:50051", stack="prod", app="reporter")
    blob = c.bind("uploads")              # declaration-driven
    mail = c.optional("mail")             # None -> run with the feature off
    key, value = blob.header()            # X-Edge identity for adapter calls
"""
import os
import sys
from dataclasses import dataclass

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "_gen"))

import grpc  # noqa: E402
from redstone.core.v1 import core_pb2, core_pb2_grpc  # noqa: E402

_DEADLINE = float(os.environ.get("REDSTONE_BIND_DEADLINE_S", "8"))


class BindRefused(RuntimeError):
    """The register said no - the message carries its written reasons."""


@dataclass
class Binding:
    name: str
    capability: str
    effective_level: str
    verified: bool
    endpoint: str
    public: str
    task: str
    _edge: str

    def header(self):
        """Edge identity for capability calls: ("X-Edge", "stack/app/task")."""
        return ("X-Edge", self._edge)


class Client:
    """Binds capabilities for one app in one stack. Lazy channel."""

    def __init__(self, register="register:50051", stack="", app=""):
        self.stack, self.app = stack, app
        self._stub = core_pb2_grpc.RegisterServiceStub(grpc.insecure_channel(register))

    def bind(self, task, capability="", level="", name="", labels=None):
        """Zero kwargs = declaration-driven (the stack file decides)."""
        request = core_pb2.BindRequest(**{
            "capability": capability, "level": level, "name": name,
            "labels": labels or {}, "stack": self.stack,
            "consumer": self.app, "as": task,  # `as` is a Python keyword
        })
        try:
            r = self._stub.Bind(request, timeout=_DEADLINE)
        except grpc.RpcError as err:
            raise BindRefused(f"bind {self.stack}/{self.app}.{task}: {err.details()}") from None
        return Binding(r.name, r.capability, r.effective_level, r.verified,
                       r.endpoint, r.public, task,
                       f"{self.stack}/{self.app}/{task}")

    def optional(self, task, **kwargs):
        """Unresolved -> None, never an exception - the feature is off."""
        try:
            return self.bind(task, **kwargs)
        except BindRefused:
            return None
