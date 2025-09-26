#!/usr/bin/env python

from setuptools import setup, find_packages

setup(
    name="ansible-operator-http-events",
    version="1.0.0",
    author="Operator Framework",
    description="HTTP event emitter for ansible-runner used by ansible-operator",
    packages=find_packages(),
    install_requires=[
        'requests',
        'requests-unixsocket',
    ],
    entry_points={
        'ansible_runner.plugins': 'http = ansible_runner_http'
    },
    zip_safe=False,
)
