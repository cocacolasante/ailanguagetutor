import * as FileSystem from 'expo-file-system';
import { API_BASE, ENDPOINTS } from '@/constants/api';
import { getToken } from '@/utils/storage';

export const fetchTTS = async (text: string, language: string): Promise<string> => {
  const token = await getToken();

  const response = await fetch(`${API_BASE}${ENDPOINTS.tts}`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify({ text, language }),
  });

  if (!response.ok) throw new Error('TTS request failed');

  const arrayBuffer = await response.arrayBuffer();
  const uint8Array = new Uint8Array(arrayBuffer);

  // Convert binary to base64 for FileSystem.writeAsStringAsync
  let binary = '';
  uint8Array.forEach((b) => { binary += String.fromCharCode(b); });
  const base64 = btoa(binary);

  const fileUri = `${FileSystem.cacheDirectory}tts_${Date.now()}.mp3`;
  await FileSystem.writeAsStringAsync(fileUri, base64, {
    encoding: FileSystem.EncodingType.Base64,
  });

  return fileUri;
};
