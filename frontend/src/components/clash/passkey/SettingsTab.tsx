import { useEffect, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { FormProvider, useForm } from 'react-hook-form';
import {
  Button,
  ConfigProvider,
  Empty,
  Flex,
  Form,
  Input,
  Modal,
  Space,
  Table,
  Typography,
  message,
  theme,
} from 'antd';
import { FormField, rhfZodValidate } from '@/components/form/rhf';
import { keys } from '@/api/queryKeys';
import {
  PasskeyDomainSchema,
  PasskeyNameSchema,
  type PasskeyDomain,
  type PasskeyName,
} from '@/schemas/clash/passkey';
import {
  fetchPasskeys,
  passkeyError,
  passkeyPost,
  passkeySupported,
  preparePasskey,
  registerPasskey,
} from '@/lib/clash/passkey/api';

const passkeyFontFamily =
  '-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei UI", "Microsoft YaHei", sans-serif';

export default function PasskeyTab() {
  const { t } = useTranslation();
  const { token } = theme.useToken();
  const domainInputId = useId();
  const [modal, modalHolder] = Modal.useModal();
  const [messageApi, messageHolder] = message.useMessage();
  const cache = useQueryClient();
  const query = useQuery({ queryKey: keys.settings.passkeys(), queryFn: fetchPasskeys });
  const domainForm = useForm<PasskeyDomain>({
    defaultValues: { domain: window.location.hostname },
  });
  const nameForm = useForm<PasskeyName>({ defaultValues: { name: '' } });
  const [pending, setPending] = useState<Awaited<ReturnType<typeof preparePasskey>> | null>(null);
  const [preparing, setPreparing] = useState(false);
  const { reset } = domainForm;
  const domain = query.data?.domain;
  useEffect(() => {
    if (domain !== undefined) reset({ domain: domain || window.location.hostname });
  }, [domain, reset]);
  useEffect(() => {
    if (query.error) messageApi.error(passkeyError(query.error));
  }, [query.error, messageApi]);
  const mutation = useMutation({
    mutationFn: (operation: () => Promise<unknown>) => operation(),
    onSuccess: async () => {
      messageApi.success(t('success'));
      await cache.invalidateQueries({ queryKey: keys.settings.passkeys() });
    },
    onError: (error) => {
      messageApi.error(passkeyError(error));
    },
  });
  const busy = mutation.isPending || preparing;

  async function addPasskey() {
    if (!passkeySupported()) {
      messageApi.error(t('passkey.unsupported'));
      return;
    }
    if (domain !== window.location.hostname) {
      messageApi.error(t('passkey.invalidOrigin'));
      return;
    }
    setPreparing(true);
    try {
      const registration = await preparePasskey();
      nameForm.reset();
      setPending(registration);
    } catch (error) {
      messageApi.error(passkeyError(error));
    } finally {
      setPreparing(false);
    }
  }

  function saveDomain(values: PasskeyDomain) {
    const payload = PasskeyDomainSchema.parse(values);
    if (domain && domain !== payload.domain.trim().toLowerCase()) {
      modal.confirm({
        title: t('passkey.changeDomain'),
        content: t('passkey.changeDomainWarning'),
        okText: t('confirm'),
        cancelText: t('cancel'),
        okButtonProps: { danger: true },
        onOk: () =>
          mutation.mutateAsync(() => passkeyPost('/panel/api/clash/passkeys/domain', payload)),
      });
    } else {
      mutation.mutate(() => passkeyPost('/panel/api/clash/passkeys/domain', payload));
    }
  }

  function saveName(values: PasskeyName) {
    if (!pending || mutation.isPending) return;
    const { name } = PasskeyNameSchema.parse(values);
    mutation.mutate(
      async () => {
        await registerPasskey(pending, name);
        setPending(null);
      },
      { onError: () => setPending(null) },
    );
  }

  return (
    <ConfigProvider
      theme={{
        token: {
          fontFamily: passkeyFontFamily,
          fontSize: 14,
          fontWeightStrong: 400,
          controlHeight: 32,
        },
        components: {
          Table: {
            headerColor: token.colorText,
            headerBg: token.colorFillQuaternary,
            headerSplitColor: 'transparent',
            cellFontSizeMD: 14,
            cellPaddingBlockMD: 12,
          },
        },
      }}
    >
      <Flex
        vertical
        gap={16}
        style={{
          width: '100%',
          padding: '8px 20px 16px',
          fontFamily: passkeyFontFamily,
          fontSize: 14,
          fontWeight: 400,
        }}
      >
        {modalHolder}
        {messageHolder}
        <FormProvider {...domainForm}>
          <Form layout="vertical" onFinish={domainForm.handleSubmit(saveDomain)}>
            <Form.Item
              label={t('passkey.domain')}
              htmlFor={domainInputId}
              style={{ marginBottom: 8 }}
            >
              <Flex gap="small" align="start">
                <div style={{ flex: '0 1 360px', minWidth: 0 }}>
                  <FormField
                    name="domain"
                    style={{ marginBottom: 0 }}
                    rules={{ validate: rhfZodValidate(PasskeyDomainSchema.shape.domain) }}
                  >
                    <Input
                      id={domainInputId}
                      autoComplete="off"
                      placeholder="panel.example.com"
                      disabled={busy || query.isPending}
                    />
                  </FormField>
                </div>
                <Space>
                  <Button
                    onClick={() =>
                      domainForm.setValue('domain', window.location.hostname, {
                        shouldDirty: true,
                        shouldValidate: true,
                      })
                    }
                    disabled={busy}
                  >
                    {t('passkey.detectDomain')}
                  </Button>
                  <Button type="primary" htmlType="submit" loading={busy} disabled={!query.data}>
                    {t('save')}
                  </Button>
                </Space>
              </Flex>
            </Form.Item>
          </Form>
        </FormProvider>
        <Flex justify="space-between" align="center">
          <Typography.Text>{t('passkey.title')}</Typography.Text>
          <Button
            type="primary"
            disabled={!query.data || !!pending}
            loading={busy}
            onClick={addPasskey}
          >
            {t('passkey.add')}
          </Button>
        </Flex>
        <Modal
          open={!!pending}
          title={t('passkey.name')}
          okText={t('save')}
          cancelText={t('cancel')}
          confirmLoading={mutation.isPending}
          closable={!mutation.isPending}
          mask={{ closable: false }}
          cancelButtonProps={{ disabled: mutation.isPending }}
          onCancel={() => setPending(null)}
          onOk={nameForm.handleSubmit(saveName)}
        >
          <FormProvider {...nameForm}>
            <Form layout="vertical" onFinish={nameForm.handleSubmit(saveName)}>
              <FormField
                name="name"
                label={t('passkey.name')}
                rules={{ validate: rhfZodValidate(PasskeyNameSchema.shape.name) }}
              >
                <Input maxLength={64} autoComplete="off" disabled={busy || !query.data} />
              </FormField>
            </Form>
          </FormProvider>
        </Modal>
        <Table
          size="middle"
          locale={{ emptyText: <Empty image={false} style={{ marginBlock: 28 }} /> }}
          rowKey="id"
          loading={query.isPending}
          dataSource={query.data?.credentials}
          pagination={false}
          scroll={{ x: 540 }}
          columns={[
            { title: t('passkey.name'), dataIndex: 'name' },
            {
              title: t('passkey.createdAt'),
              dataIndex: 'createdAt',
              render: (value: number) => new Date(value).toLocaleString(),
            },
            {
              title: t('passkey.lastUsedAt'),
              dataIndex: 'lastUsedAt',
              render: (value: number) =>
                value ? new Date(value).toLocaleString() : t('passkey.neverUsed'),
            },
            {
              title: t('remove'),
              key: 'actions',
              render: (_, row) => (
                <Button
                  danger
                  disabled={busy}
                  onClick={() =>
                    modal.confirm({
                      title: t('passkey.delete'),
                      content: t('passkey.deleteWarning', { name: row.name }),
                      okText: t('remove'),
                      cancelText: t('cancel'),
                      okButtonProps: { danger: true },
                      onOk: () =>
                        mutation.mutateAsync(() =>
                          passkeyPost('/panel/api/clash/passkeys/delete', { id: row.id }),
                        ),
                    })
                  }
                >
                  {t('remove')}
                </Button>
              ),
            },
          ]}
        />
      </Flex>
    </ConfigProvider>
  );
}
