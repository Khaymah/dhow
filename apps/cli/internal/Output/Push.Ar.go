package output

var arPushMessages = map[Key]string{ //nolint:gosec // Messages, not credentials.
	KeyPushUsage: `الاستخدام:
  dhow artifact push [options] <folder | file.tar.gz>

يعبّئ مجلدًا، أو يأخذ ملف .tar.gz بنيته من قبل، ويرفعه إلى سيرفر داو.
يحفظ داو الملف باسم بصمته SHA-256، ويبدأ تشغيلًا يستخدم هذه البايتات نفسها.

الخيارات:
  --server URL        سيرفر داو (أو اضبط DHOW_SERVER)
  --name NAME         اسم الملف الناتج، كما يكتبه الـ pipeline تحت ingested
  --ref REF           الفرع أو العلامة التي بُنيت
  --ref-kind KIND     branch (الافتراضي) أو tag
  --sha COMMIT        الـ commit الذي بُني
  --provenance K=V    معلومة عن الـ CI، مثل Repository=Khaymah/dhow (كرّره عند الحاجة)
  --no-trigger        احفظ الملف ولا تبدأ تشغيلًا
  --token-stdin       اقرأ توكن الاستقبال من الدخل القياسي
  --ca-file FILE      ملف PEM فيه جهات اعتماد إضافية (أو اضبط DHOW_CA_FILE)
  --retries N         عدد إعادات المحاولة بعد فشل مؤقت، من 0 إلى 10 (الافتراضي 3)
  --dry-run           عبّئ، ثم اطبع البصمة والترويسات، ولا ترسل شيئًا
  --format text|json  شكل النتيجة
  --lang en|ar        لغة الرسائل

يأتي التوكن من DHOW_INGEST_TOKEN أو من --token-stdin فقط. لا يُقرأ من خيار في سطر الأوامر،
لأن الخيارات تظهر في قوائم العمليات وفي السجلات. يحترم الأمر HTTPS_PROXY وNO_PROXY.
لا يقبل الأمر http:// إلا مع localhost، حتى لا يمر التوكن في الشبكة نصًا واضحًا.

أكواد الخروج:
  0  حُفظ الملف (201)، أو كان محفوظًا من قبل (200)
  1  رفض السيرفر الملف أو بياناته
  2  الأمر أو مدخلاته غير صحيحة، ولم يُرسل شيء
  3  لا يقبل السيرفر التوكن، أو لا يقبله لهذا الـ ref
  4  بقي فشل مؤقت بعد إعادة المحاولة
  5  الرد ليس ما يرسله سيرفر داو
`,
	KeyPushPrefix:            "dhow artifact push:",
	KeyPushUnknownSubcommand: "dhow artifact: %s ليس أمرًا. استخدم dhow artifact push.",
	KeyPushUnknownOption:     "dhow artifact push: %s ليس خيارًا.",
	KeyPushOptionValue:       "dhow artifact push: يحتاج %s إلى قيمة.",
	KeyPushTokenFlag:         "dhow artifact push: لا يُقرأ التوكن من خيار في سطر الأوامر، لأن الخيارات تظهر في قوائم العمليات وفي السجلات. اضبط DHOW_INGEST_TOKEN، أو استخدم --token-stdin.",
	KeyPushProvenanceArg:     "dhow artifact push: يحتاج --provenance إلى Key=Value، مثل --provenance Repository=Khaymah/dhow.",
	KeyPushRetries:           "dhow artifact push: قيمة --retries رقم من 0 إلى 10.",
	KeyPushNeedPath:          "dhow artifact push: اكتب مجلدًا واحدًا أو ملف .tar.gz واحدًا.",
	KeyPushRequired:          "dhow artifact push: الخيار %s مطلوب (ويصلح DHOW_SERVER للسيرفر).",
	KeyPushNeedToken:         "dhow artifact push: لا يوجد توكن. اضبط DHOW_INGEST_TOKEN، أو استخدم --token-stdin وأرسل التوكن في الدخل القياسي.",
	KeyPushTokenRead:         "dhow artifact push: لا يمكن قراءة التوكن من الدخل القياسي.",
	KeyPushTokenShape:        "dhow artifact push: شكل التوكن ليس dhow_ing_ ومعه 43 حرفًا. راجع السر الذي مررته.",
	KeyPushLangInvalid:       "dhow artifact push: قيمة --lang هي en أو ar.",
	KeyPushFormatInvalid:     "dhow artifact push: قيمة --format هي text أو json.",
	KeyPushRetrying:          "فشلت المحاولة %d من %d: %s. ستعاد المحاولة بعد %s.",
	KeyPushRetryAfter:        "يطلب السيرفر أن تنتظر %s ثانية.",
	KeyPushAttempts:          "جرى الرفع %d مرات.",
	KeyPushClassToken:        "لا يقبل السيرفر هذا التوكن لهذا الرفع.",
	KeyPushClassRefused:      "رفض السيرفر الرفع.",
	KeyPushClassTemporary:    "لا يستطيع السيرفر استقبال الرفع الآن.",
	KeyPushClassUnexpected:   "رد السيرفر بما ليس ردًا من داو.",
	KeyPushErrServer:         "عنوان السيرفر يجب أن يبدأ بـ https://، أو http://localhost، دون مستخدم أو استعلام أو جزء.",
	KeyPushErrName:           "الخيار --name حروف وأرقام ونقطة وشرطة سفلية وشرطة، ويبدأ بحرف أو رقم، وطوله 64 حرفًا على الأكثر.",
	KeyPushErrRef:            "قيمة --ref ليست اسم فرع أو علامة صحيحًا مع قيمة --ref-kind التي كتبتها.",
	KeyPushErrRefKind:        "قيمة --ref-kind هي branch أو tag.",
	KeyPushErrSha:            "يجب أن يكون --sha رقم commit كاملًا بأحرف hex صغيرة (40 أو 64 حرفًا)، وليس كله أصفارًا.",
	KeyPushErrProvenance:     "يسمح --provenance بـ 16 حقلًا على الأكثر. المفتاح يبدأ بحرف، ويستخدم حروفًا وأرقامًا وشرطة سفلية، وطوله 32 حرفًا على الأكثر. والقيمة نص عادي بطول 256 حرفًا على الأكثر.",
	KeyPushErrCAFile:         "لا يمكن قراءة ملف الشهادات، أو لا توجد فيه شهادة بصيغة PEM.",
	KeyPushErrNotGzip:        "الملف ليس أرشيف .tar.gz.",
	KeyPushErrEmpty:          "لا يوجد ما يُرسل: المجلد أو الملف فارغ.",
	KeyPushErrUnsafe:         "الأرشيف غير آمن: فيه مسار خارج المجلد، أو ملف جهاز، أو رابط يخرج من المجلد. سيرفضه السيرفر.",
	KeyPushErrUnsupported:    "في المجلد ملف جهاز، أو socket، أو رابط يخرج من المجلد. احذفه، أو عبّئ ما تنشره فقط.",
	KeyPushErrMissing:        "مسار في المجلد غير موجود.",
	KeyPushErrTooLarge:       "في الأرشيف إدخالات كثيرة أو حجمه أكبر من أن يُفك.",
	KeyPushErrPath:           "اسم في المجلد ليس مسارًا نسبيًا آمنًا.",
	KeyPushErrSource:         "لا يمكن قراءة المسار كمجلد أو كملف .tar.gz (%s).",
	KeyPushDryRun:            "تشغيل تجريبي: لم يُرسل شيء.",
	KeyPushDigest:            "البصمة: sha256:%s",
	KeyPushSize:              "الحجم: %d بايت",
	KeyPushStored:            "حُفظ %s باسم sha256:%s (%d بايت).",
	KeyPushDuplicate:         "محفوظ من قبل: %s باسم sha256:%s (%d بايت). لم يتغير شيء.",
	KeyPushRun:               "بدأ التشغيل رقم %d: %s",
	KeyPushNoRun:             "لم يبدأ تشغيل (%s).",

	CodeKey("INGEST_TOKEN_INVALID"):      "لا يعرف هذا السيرفر التوكن. تأكد أنه التوكن كاملًا، وأن عنوان السيرفر صحيح.",
	CodeKey("INGEST_TOKEN_REVOKED"):      "أُلغي التوكن. أنشئ توكن استقبال جديدًا من إعدادات المشروع.",
	CodeKey("INGEST_TOKEN_EXPIRED"):      "انتهت مدة التوكن. أنشئ توكن استقبال جديدًا من إعدادات المشروع.",
	CodeKey("REF_NOT_ALLOWED_FOR_TOKEN"): "لا يسمح هذا التوكن بالرفع لهذا الـ ref. راجع أنماط الـ ref في التوكن.",
	CodeKey("DIGEST_MISMATCH"):           "البايتات التي وصلت إلى السيرفر لا تطابق بصمتها. تغيّر الملف أو تلف في الطريق. شغّل الأمر مرة أخرى.",
	CodeKey("ARCHIVE_UNSAFE"):            "الأرشيف غير آمن للفك: فيه مسار خارج المجلد، أو ملف جهاز، أو رابط يخرج من المجلد.",
	CodeKey("ARTIFACT_TOO_LARGE"):        "الملف أكبر مما يقبله هذا السيرفر (DHOW_INGEST_MAX_BYTES).",
	CodeKey("ARTIFACT_QUOTA_EXCEEDED"):   "لم يعد في المؤسسة مكان للملفات الناتجة. انتظر حتى تنتهي مدة الملفات القديمة، أو اطلب من المسؤول.",
	CodeKey("LENGTH_REQUIRED"):           "يحتاج السيرفر إلى حجم الملف أولًا. قد يكون وكيل في الطريق غيّر الطلب.",
	CodeKey("UPLOAD_TOO_SLOW"):           "كان الرفع بطيئًا جدًا، فقطعه السيرفر.",
	CodeKey("INGEST_TOKEN_QUOTA"):        "رفع هذا التوكن أكثر ما يسمح له به. ألغِه وأنشئ توكنًا جديدًا.",
	CodeKey("RATE_LIMITED"):              "رفعات كثيرة. انتظر ثم شغّل الأمر مرة أخرى.",
	CodeKey("SERVICE_UNAVAILABLE"):       "السيرفر مشغول برفعات أخرى.",
	CodeKey("VALIDATION_FAILED"):         "لا يقبل السيرفر أحد البيانات التي أرسلتها (الاسم أو الـ ref أو الـ commit أو provenance).",
	CodeKey("PROJECT_ARCHIVED"):          "المشروع مؤرشف.",
	CodeKey("NETWORK"):                   "فشل الاتصال بالسيرفر.",
	CodeKey("TIMEOUT"):                   "استغرق الرفع وقتًا أطول من المسموح.",
	CodeKey("REDIRECT"):                  "وجّه السيرفر الطلب إلى مكان آخر. لا يتبع الأمر أي تحويل، حتى يبقى التوكن عند السيرفر الذي سمّيته. اكتب العنوان النهائي في --server.",
	CodeKey("BAD_RESPONSE"):              "الرد ليس رد داو. تأكد أن --server يشير إلى داو، وليس إلى صفحة دخول أو وكيل.",
	CodeKey("RESPONSE_DIGEST_DIFFERS"):   "يقول السيرفر إنه حفظ بصمة غير التي أرسلناها.",
	CodeKey("HTTP_4XX"):                  "رفض السيرفر الطلب.",
	CodeKey("HTTP_5XX"):                  "فشل السيرفر في معالجة الطلب.",
	CodeKey("HTTP_UNEXPECTED"):           "أرسل السيرفر ردًا لا يعرفه هذا الأمر.",
	CodeKey("HTTP_413"):                  "يقول وكيل أو السيرفر إن الملف كبير جدًا. ارفع حد الجسم للمسار /ingest/ في الوكيل العكسي.",
	CodeKey("TLS"):                       "شهادة السيرفر غير موثوقة. إذا كان لشركتك جهة اعتماد خاصة، فمرّر ملف PEM الخاص بها بـ --ca-file أو DHOW_CA_FILE.",
}
