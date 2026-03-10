import { useState, useRef } from 'react';
import { Audio } from 'expo-av';
import { fetchTTS } from '@/api/tts';

export function useAudio() {
  const [isPlaying, setIsPlaying] = useState(false);
  const soundRef = useRef<Audio.Sound | null>(null);

  const stopAudio = async () => {
    if (soundRef.current) {
      await soundRef.current.stopAsync();
      await soundRef.current.unloadAsync();
      soundRef.current = null;
    }
    setIsPlaying(false);
  };

  const playTTS = async (text: string, language: string) => {
    try {
      await stopAudio();

      await Audio.setAudioModeAsync({
        playsInSilentModeIOS: true,
        allowsRecordingIOS: false,
      });

      const uri = await fetchTTS(text, language);
      const { sound } = await Audio.Sound.createAsync({ uri });
      soundRef.current = sound;

      setIsPlaying(true);
      await sound.playAsync();

      sound.setOnPlaybackStatusUpdate((status) => {
        if (status.isLoaded && status.didJustFinish) {
          setIsPlaying(false);
          sound.unloadAsync();
          soundRef.current = null;
        }
      });
    } catch (err) {
      setIsPlaying(false);
      console.error('TTS playback error:', err);
    }
  };

  return { playTTS, stopAudio, isPlaying };
}
