-- Synthetic integration fixtures, not verified addresses or organizer data.
CREATE TABLE public.orders (id bigint PRIMARY KEY, address text);
INSERT INTO public.orders VALUES
 (9223372036854775807, E'  Астана, Туран 55/6, кв 77\nвход со двора  '),
 (2, 'Неизвестный адрес'),
 (3, NULL),
 (4, ''),
 (5, 'Неизвестный адрес');
CREATE TABLE public.changed_orders (id uuid PRIMARY KEY, address text);
INSERT INTO public.changed_orders VALUES ('12345678-1234-1234-1234-123456789abc','Синтетический неизвестный адрес');
CREATE TABLE public.unrelated (id text PRIMARY KEY);
