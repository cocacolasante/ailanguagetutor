import { File, Paths } from 'expo-file-system';
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
  const file = new File(Paths.cache, `tts_${Date.now()}.mp3`);
  const writer = file.writableStream().getWriter();
  await writer.write(new Uint8Array(arrayBuffer));
  await writer.close();

  return file.uri;
};
