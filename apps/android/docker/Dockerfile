# Android build environment for the Kinora APK (JDK 21 + Android SDK 36 + Node 22).
# Nothing has to be installed on the host: apps/android/build.sh runs Gradle inside this image.
FROM node:22.11.0-bookworm-slim AS node

FROM eclipse-temurin:21.0.5_11-jdk-jammy
ENV ANDROID_HOME=/opt/android-sdk ANDROID_SDK_ROOT=/opt/android-sdk \
    GRADLE_USER_HOME=/gradle
RUN apt-get update && apt-get install -y --no-install-recommends unzip curl ca-certificates git && rm -rf /var/lib/apt/lists/*
COPY --from=node /usr/local /usr/local
ARG CMDLINE_TOOLS=commandlinetools-linux-11076708_latest.zip
RUN mkdir -p $ANDROID_HOME/cmdline-tools && cd /tmp \
 && curl -fsSL -o tools.zip https://dl.google.com/android/repository/$CMDLINE_TOOLS \
 && unzip -q tools.zip -d $ANDROID_HOME/cmdline-tools && mv $ANDROID_HOME/cmdline-tools/cmdline-tools $ANDROID_HOME/cmdline-tools/latest \
 && rm tools.zip \
 && yes | $ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager --licenses >/dev/null \
 && $ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager "platform-tools" "platforms;android-36" "build-tools;36.0.0" >/dev/null
ENV PATH=$PATH:$ANDROID_HOME/platform-tools:$ANDROID_HOME/build-tools/36.0.0
WORKDIR /src
