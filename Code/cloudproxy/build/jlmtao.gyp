{
  'target_defaults': {
    'product_dir': 'bin',
  },
  'variables': {
    'base': '../../',
    'ac': '<(base)/accessControl',
    'ch': '<(base)/channels',
    'cl': '<(base)/claims',
    'cm': '<(base)/commonCode',
    'fp': '<(base)/fileProxy',
    'kn': '<(base)/keyNegoServer',
    'jb': '<(base)/jlmbignum',
    'jc': '<(base)/jlmcrypto',
    'pr': '<(base)/protocolChannel',
    'ta': '<(base)/tao',
    'tc': '<(base)/tcService',
    'tp': '<(base)/TPMDirect',
    'vt': '<(base)/vault',
  },
  'targets': [
    {
        'target_name': 'bignum',
        'type': 'static_library',
        'include_dirs': [
            '<(cm)',
            '<(jc)',
            '<(jb)',
        ],
        'sources': [
            '<(jb)/mpBasicArith.cpp',
            '<(jb)/mpModArith.cpp',
	        '<(jb)/mpNumTheory.cpp',
	        '<(jb)/fastArith.cpp',
        ],
    },
    {
        'target_name': 'bignum_O1',
        'type': 'static_library',
        'include_dirs': [
            '<(cm)',
            '<(jc)',
            '<(jb)',
        ],
        'cflags': [
            '-Wall',
            '-Werror',
            '-Wno-unknown-pragmas',
            '-Wno-format',
            '-O1',
        ],
        'defines': [
            'TPMSUPPORT',
            'QUOTE2_DEFINED',
            'TEST',
            '__FLUSHIO__',
            'ENCRYPTTHENMAC',
        ],
        'sources': [
            '<(jb)/mpBasicArith.cpp',
            '<(jb)/mpModArith.cpp',
	        '<(jb)/mpNumTheory.cpp',
	        '<(jb)/fastArith.cpp',
        ],
    },
    {
        'target_name': 'keyNegoServer',
        'type': 'executable',
        'cflags': [
            '-Wall',
            '-Werror',
            '-Wno-unknown-pragmas',
            '-Wno-format',
            '-O3',
        ],
        'libraries': [
            '-lpthread',
        ],
        'defines': [
            'FILECLIENT',
            'LINUX',
            'QUOTE2_DEFINED',
            'TEST',
            '__FLUSHIO__',
        ],
        'include_dirs': [
            '<(kn)',
            '<(cm)',
            '<(fp)',
            '<(jc)',
            '<(ac)',
            '<(jb)',
            '<(cl)',
            '<(ta)',
            '<(ch)',
            '<(tp)',
            '<(vt)',
            '<(tc)',
        ],
        'sources': [
            '<(kn)/keyNegoServer.cpp',
            '<(cm)/logging.cpp',
            '<(jc)/jlmcrypto.cpp',
            '<(cm)/jlmUtility.cpp',
            '<(jc)/keys.cpp',
            '<(jc)/aesni.cpp',
            '<(jc)/sha256.cpp',
            '<(jc)/sha1.cpp',
            '<(ch)/channel.cpp',
            '<(jc)/hmacsha256.cpp',
            '<(tp)/hashprep.cpp',
            '<(jc)/cryptoHelper.cpp',
            '<(cl)/quote.cpp',
            '<(cl)/cert.cpp',
            '<(fp)/resource.cpp',
            '<(jc)/modesandpadding.cpp',
            '<(cl)/validateEvidence.cpp',
            '<(cm)/tinystr.cpp',
            '<(cm)/tinyxmlerror.cpp',
            '<(cm)/tinyxml.cpp',
            '<(cm)/tinyxmlparser.cpp',
            '<(jc)/encryptedblockIO.cpp',
        ],
        'dependencies': [
            'bignum',
        ],
    },
    {
        'target_name': 'tcService',
        'type': 'executable',
        'cflags': [
            '-Wall',
            '-Werror',
            '-Wno-unknown-pragmas',
            '-Wno-format',
            '-O3',
        ],
        'libraries': [
            '-lpthread',
        ],
        'defines': [
            'TPMSUPPORT',
            'QUOTE2_DEFINED',
            'TEST',
            '__FLUSHIO__',
            'ENCRYPTTHENMAC',
        ],
        'include_dirs': [
            '<(tc)',
            '<(ta)',
            '<(cm)',
            '<(jc)',
            '<(jb)',
            '<(tp)',
            '<(ch)',
            '<(vt)',
            '<(cl)',
            '<(fp)',
        ],
        'sources': [
            '<(tc)/tcIO.cpp',
            '<(cm)/logging.cpp',
            '<(jc)/jlmcrypto.cpp',
            '<(cm)/jlmUtility.cpp',
            '<(jc)/keys.cpp',
            '<(jc)/aesni.cpp',
            '<(jc)/sha256.cpp',
            '<(jc)/cryptoHelper.cpp',
            '<(jc)/fileHash.cpp',
            '<(jc)/hmacsha256.cpp',
            '<(jc)/modesandpadding.cpp',
            '<(tc)/buffercoding.cpp',
            '<(ta)/taoSupport.cpp',
            '<(ta)/taoEnvironment.cpp',
            '<(ta)/taoHostServices.cpp',
            '<(ta)/taoInit.cpp',
            '<(ta)/linuxHostsupport.cpp',
            '<(ta)/TPMHostsupport.cpp',
            '<(jc)/sha1.cpp',
            '<(cm)/tinystr.cpp',
            '<(cm)/tinyxmlerror.cpp',
            '<(fp)/resource.cpp',
            '<(cm)/tinyxml.cpp',
            '<(cm)/tinyxmlparser.cpp',
            '<(tp)/vTCIDirect.cpp',
            '<(vt)/vault.cpp',
            '<(tc)/tcService.cpp',
            '<(tp)/hmacsha1.cpp',
            '<(cl)/cert.cpp',
            '<(ta)/trustedKeyNego.cpp',
            '<(cl)/quote.cpp',
            '<(ch)/channel.cpp',
            '<(tp)/hashprep.cpp',
            '<(jc)/encryptedblockIO.cpp',
        ],
        'dependencies': [
            'bignum_O1',
        ],
    },
    {
        'target_name': 'fileServer',
        'type': 'executable',
        'cflags': [
            '-Wall',
            '-Werror',
            '-Wno-unknown-pragmas',
            '-Wno-format',
            '-O3',
        ],
        'libraries': [
            '-lpthread',
        ],
        'defines': [
            'LINUX',
            'TEST',
            '__FLUSHIO__',
            'ENCRYPTTHENMAC',
        ],
        'include_dirs': [
            '<(fp)',
            '<(cm)',
            '<(jc)',
            '<(jb)',
            '<(cl)',
            '<(pr)',
            '<(ac)',
            '<(ta)',
            '<(vt)',
            '<(tc)',
            '<(tp)',
            '<(ch)',
        ],
        'sources': [
            '<(fp)/fileServer.cpp',
            '<(jc)/jlmcrypto.cpp',
            '<(tp)/hashprep.cpp',
            '<(pr)/session.cpp',
            '<(pr)/request.cpp',
            '<(cm)/jlmUtility.cpp',
            '<(jc)/keys.cpp',
            '<(jc)/aesni.cpp',
            '<(jc)/sha256.cpp',
            '<(jc)/cryptoHelper.cpp',
            '<(cl)/cert.cpp',
            '<(cl)/quote.cpp',
            '<(cl)/validateEvidence.cpp',
            '<(fp)/resource.cpp',
            '<(ac)/accessControl.cpp',
            '<(ac)/signedAssertion.cpp',
            '<(jc)/encryptedblockIO.cpp',
            '<(fp)/fileServices.cpp',
            '<(jc)/hmacsha256.cpp',
            '<(jc)/modesandpadding.cpp',
            '<(ta)/trustedKeyNego.cpp',
            '<(ta)/taoSupport.cpp',
            '<(ta)/taoEnvironment.cpp',
            '<(ta)/taoHostServices.cpp',
            '<(ta)/taoInit.cpp',
            '<(ta)/linuxHostsupport.cpp',
            '<(cm)/tinystr.cpp',
            '<(cm)/tinyxmlerror.cpp',
            '<(cm)/tinyxml.cpp',
            '<(ch)/channel.cpp',
            '<(ch)/safeChannel.cpp',
            '<(cm)/tinyxmlparser.cpp',
            '<(jc)/sha1.cpp',
            '<(cm)/logging.cpp',
            '<(vt)/vault.cpp',
            '<(tc)/buffercoding.cpp',
            '<(tc)/tcIO.cpp',
        ],
        'dependencies': [
            'bignum_O1',
        ],
    },
    {
        'target_name': 'fileClient',
        'type': 'executable',
        'cflags': [
            '-Wall',
            '-Werror',
            '-Wno-unknown-pragmas',
            '-Wno-format',
            '-O3',
        ],
        'libraries': [
            '-lpthread',
        ],
        'defines': [
            'LINUX',
            'FILECLIENT',
            'TEST',
            'TIXML_USE_STL',
            '__FLUSHIO__',
            'ENCRYPTTHENMAC',
        ],
        'include_dirs': [
            '<(fp)',
            '<(cm)',
            '<(jc)',
            '<(jb)',
            '<(cl)',
            '<(ta)',
            '<(tc)',
            '<(tp)',
            '<(ch)',
            '<(pr)',
            '<(ac)',
            '<(vt)',
        ],
        'sources': [
            '<(cm)/jlmUtility.cpp',
            '<(jc)/keys.cpp',
            '<(jc)/cryptoHelper.cpp',
            '<(jc)/jlmcrypto.cpp',
            '<(jc)/aesni.cpp',
            '<(jc)/sha256.cpp',
            '<(jc)/sha1.cpp',
            '<(jc)/hmacsha256.cpp',
            '<(jc)/encryptedblockIO.cpp',
            '<(jc)/modesandpadding.cpp',
            '<(ta)/taoSupport.cpp',
            '<(ta)/taoEnvironment.cpp',
            '<(ta)/taoHostServices.cpp',
            '<(ta)/taoInit.cpp',
            '<(ta)/linuxHostsupport.cpp',
            '<(cl)/cert.cpp',
            '<(cl)/quote.cpp',
            '<(cm)/tinyxml.cpp',
            '<(cm)/tinyxmlparser.cpp',
            '<(cm)/tinystr.cpp',
            '<(cm)/tinyxmlerror.cpp',
            '<(ch)/channel.cpp',
            '<(ch)/safeChannel.cpp',
            '<(pr)/session.cpp',
            '<(pr)/request.cpp',
            '<(fp)/fileServices.cpp',
            '<(cl)/validateEvidence.cpp',
            '<(ta)/trustedKeyNego.cpp',
            '<(tc)/buffercoding.cpp',
            '<(tc)/tcIO.cpp',
            '<(tp)/hashprep.cpp',
            '<(fp)/fileTester.cpp',
            '<(fp)/fileClient.cpp',
            '<(cm)/logging.cpp',
        ],
        'dependencies': [
            'bignum_O1',
        ],
    },
  ]
}
