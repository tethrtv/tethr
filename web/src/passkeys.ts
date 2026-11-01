import { startAuthentication, startRegistration } from '@simplewebauthn/browser'
import { api } from './api/client'

export const passkeysSupported = () => typeof window !== 'undefined' && 'PublicKeyCredential' in window

export async function addPasskey(name: string) {
  const { data: options, error } = await api.POST('/v1/passkeys/options')
  if (error || !options) throw error ?? new Error('could not start')
  // biome-ignore lint/suspicious/noExplicitAny: the options are WebAuthn's own JSON shape
  const credential = await startRegistration({ optionsJSON: options.options as any })
  const { data, error: saveError } = await api.POST('/v1/passkeys', {
    body: { challenge: options.challenge, credential: credential as unknown as Record<string, unknown>, name },
  })
  if (saveError) throw saveError
  return data
}

export async function signInWithPasskey() {
  const { data: options, error } = await api.POST('/v1/auth/passkey/options')
  if (error || !options) throw error ?? new Error('could not start')
  // biome-ignore lint/suspicious/noExplicitAny: the options are WebAuthn's own JSON shape
  const credential = await startAuthentication({ optionsJSON: options.options as any })
  const { data, error: signInError } = await api.POST('/v1/auth/passkey', {
    body: { challenge: options.challenge, credential: credential as unknown as Record<string, unknown> },
  })
  if (signInError) throw signInError
  return data
}
