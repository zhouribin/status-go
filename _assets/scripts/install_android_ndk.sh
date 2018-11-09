#!/bin/bash		
		
curl https://dl.google.com/android/repository/android-ndk-r17b-linux-x86_64.zip -o android-ndk-r17b.zip		
unzip -q android-ndk-r17b.zip && rm android-ndk-r17b.zip		
mv android-ndk-r17b $HOME		
export ANDROID_NDK=$HOME/android-ndk-r17b
export ANDROID_HOME=$HOME/android-ndk-r17b
