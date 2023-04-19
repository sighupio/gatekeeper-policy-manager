FROM quay.io/sighup/gatekeeper-policy-manager:v1.0.3

# Add awscli to GPM image
USER root
WORKDIR /tmp
RUN apt-get update && apt-get install -y --no-install-recommends unzip && rm -rf /var/lib/apt/lists/*
ADD "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" /tmp
RUN unzip awscli-exe-linux-x86_64.zip && aws/install && rm -rf aws && rm awscli-exe-linux-x86_64.zip

# Go back to the original image settings
WORKDIR /app
USER nonroot
