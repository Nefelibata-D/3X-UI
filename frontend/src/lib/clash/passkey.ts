import i18next from 'i18next';
import { HttpUtil } from '@/utils';
import { PasskeyViewSchema } from '@/schemas/passkey';

const root = '/panel/api/clash/passkeys';
const jsonOptions = { silent: true, headers: { 'Content-Type': 'application/json' } };

export function passkeySupported(): boolean {
  return (
    window.isSecureContext &&
    typeof PublicKeyCredential !== 'undefined' &&
    typeof PublicKeyCredential.parseCreationOptionsFromJSON === 'function' &&
    typeof PublicKeyCredential.parseRequestOptionsFromJSON === 'function' &&
    typeof PublicKeyCredential.prototype.toJSON === 'function'
  );
}

export async function passkeyPost<T = unknown>(path: string, body?: unknown): Promise<T | null> {
  const response = await HttpUtil.post<T>(path, body, jsonOptions);
  if (!response.success) throw new Error(response.msg || i18next.t('passkey.failed'));
  return response.obj;
}

export async function fetchPasskeys() {
  const response = await HttpUtil.get(root, undefined, { silent: true });
  if (!response.success) throw new Error(response.msg || i18next.t('passkey.failed'));
  return PasskeyViewSchema.parse(response.obj);
}

export async function preparePasskey() {
  const begin = await passkeyPost<{
    requestId: string;
    options: { publicKey: PublicKeyCredentialCreationOptionsJSON };
  }>(`${root}/register/begin`);
  if (!begin) throw new Error(i18next.t('passkey.failed'));
  const credential = await navigator.credentials.create({
    publicKey: PublicKeyCredential.parseCreationOptionsFromJSON(begin.options.publicKey),
  });
  if (!(credential instanceof PublicKeyCredential)) throw new Error(i18next.t('passkey.cancelled'));
  return {
    requestId: begin.requestId,
    credential: credential.toJSON(),
  };
}

export async function registerPasskey(
  pending: Awaited<ReturnType<typeof preparePasskey>>,
  name: string,
) {
  await passkeyPost(`${root}/register/finish`, { ...pending, name });
}

export async function loginWithPasskey() {
  const begin = await passkeyPost<{
    requestId: string;
    options: { publicKey: PublicKeyCredentialRequestOptionsJSON };
  }>('/passkey/login/begin');
  if (!begin) throw new Error(i18next.t('passkey.failed'));
  const credential = await navigator.credentials.get({
    publicKey: PublicKeyCredential.parseRequestOptionsFromJSON(begin.options.publicKey),
  });
  if (!(credential instanceof PublicKeyCredential)) throw new Error(i18next.t('passkey.cancelled'));
  await passkeyPost('/passkey/login/finish', {
    requestId: begin.requestId,
    credential: credential.toJSON(),
  });
}

export function passkeyError(error: unknown): string {
  if (error instanceof DOMException) {
    if (error.name === 'NotAllowedError' || error.name === 'AbortError')
      return i18next.t('passkey.cancelled');
    if (error.name === 'InvalidStateError') return i18next.t('passkey.duplicate');
    if (error.name === 'SecurityError') return i18next.t('passkey.invalidOrigin');
  }
  return error instanceof Error ? error.message : i18next.t('passkey.failed');
}
