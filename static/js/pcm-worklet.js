/**
 * AudioWorklet processor that captures microphone input and converts it from
 * Float32 to Int16 PCM (16-bit, mono, 16kHz), then sends the raw buffer back
 * to the main thread via a transferable ArrayBuffer (zero-copy).
 *
 * This runs in a dedicated audio rendering thread for low-latency capture.
 */
class PCMCaptureProcessor extends AudioWorkletProcessor {
  process(inputs) {
    const input = inputs[0];
    if (input && input[0] && input[0].length > 0) {
      const float32 = input[0];
      const int16 = new Int16Array(float32.length);
      for (let i = 0; i < float32.length; i++) {
        // Clamp and convert float32 [-1, 1] → int16 [-32768, 32767]
        int16[i] = Math.max(-32768, Math.min(32767, float32[i] * 32768));
      }
      // Transfer ownership of the buffer to the main thread (zero-copy)
      this.port.postMessage(int16.buffer, [int16.buffer]);
    }
    return true; // keep processor alive
  }
}

registerProcessor('pcm-capture', PCMCaptureProcessor);
