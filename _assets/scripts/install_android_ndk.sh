#!/bin/bash

curl https://dl.google.com/android/repository/android-ndk-r17b-linux-x86_64.zip -o android-ndk-r17b.zip
rm -rf $HOME/android-ndk-r17
unzip -q android-ndk-r17b.zip -d $HOME && rm android-ndk-r17b.zip
export ANDROID_NDK=$HOME/android-ndk-r17b
export ANDROID_HOME=$HOME/android-ndk-r17b
