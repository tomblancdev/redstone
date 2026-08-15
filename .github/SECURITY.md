# Security

Found a way to make circuits do what they shouldn't? **Don't open a public
issue.** Mail dev@tomblanc.fr with the details; you'll get an answer within
a week.

Scope worth knowing: the kernel has **no auth by design** and is meant to
run network-internal — reports about the absence of authentication on the
kernel API are the documented posture, not a vulnerability. Everything else
(traversal, injection, verdict bypass, wire tricks) very much is.
